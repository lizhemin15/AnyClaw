package anyclaw_bridge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"mime/multipart"
	_ "image/png"
	_ "image/gif"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/identity"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/utils"

	"github.com/anyclaw/anyclaw-server/pkg/channels/pico"
)

const (
	channelName    = "anyclaw_bridge"
	connectPath    = "/containers/connect"
	pingInterval    = 30 * time.Second
	readTimeout     = 60 * time.Second
	reconnectDelay  = 2 * time.Second
	maxReconnectCap = 60 * time.Second
)

type bridgeConn struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
	closed  atomic.Bool
}

func (bc *bridgeConn) writeJSON(v any) error {
	if bc.closed.Load() {
		return fmt.Errorf("connection closed")
	}
	bc.writeMu.Lock()
	defer bc.writeMu.Unlock()
	return bc.conn.WriteJSON(v)
}

func (bc *bridgeConn) close() {
	if bc.closed.CompareAndSwap(false, true) {
		bc.conn.Close()
	}
}

// BridgeChannel implements outbound WebSocket bridge to AnyClaw-API.
// Container connects to API; messages use Pico Protocol.
type BridgeChannel struct {
	*channels.BaseChannel
	config    config.AnyClawBridgeConfig
	conn      *bridgeConn
	ctx       context.Context
	cancel    context.CancelFunc
	chatID    string
	sessionID string
}

// NewBridgeChannel creates an outbound bridge channel.
func NewBridgeChannel(cfg config.AnyClawBridgeConfig, messageBus *bus.MessageBus) (*BridgeChannel, error) {
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("anyclaw_bridge requires ANYCLAW_API_URL, ANYCLAW_INSTANCE_ID, ANYCLAW_TOKEN")
	}

	base := channels.NewBaseChannel(channelName, cfg, messageBus, nil)
	chatID := channelName + ":" + cfg.InstanceID
	return &BridgeChannel{
		BaseChannel: base,
		config:      cfg,
		chatID:      chatID,
		sessionID:   cfg.InstanceID,
	}, nil
}

// Start implements Channel. Connects outbound to API and starts read loop.
func (c *BridgeChannel) Start(ctx context.Context) error {
	logger.InfoC(channelName, "Starting AnyClaw outbound bridge")
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)

	go c.connectLoop()
	logger.InfoC(channelName, "AnyClaw outbound bridge started")
	return nil
}

// Stop implements Channel.
func (c *BridgeChannel) Stop(ctx context.Context) error {
	logger.InfoC(channelName, "Stopping AnyClaw outbound bridge")
	c.SetRunning(false)
	if c.cancel != nil {
		c.cancel()
	}
	if c.conn != nil {
		c.conn.close()
		c.conn = nil
	}
	logger.InfoC(channelName, "AnyClaw outbound bridge stopped")
	return nil
}

func (c *BridgeChannel) connectLoop() {
	backoff := reconnectDelay
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connect(); err != nil {
			logger.ErrorCF(channelName, "Bridge connection failed", map[string]any{
				"error":    err.Error(),
				"retry_in": backoff.String(),
			})
		}

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
			if backoff < maxReconnectCap {
				backoff *= 2
				if backoff > maxReconnectCap {
					backoff = maxReconnectCap
				}
			}
		}
	}
}

func (c *BridgeChannel) connect() error {
	baseURL := strings.TrimSuffix(c.config.APIURL, "/")
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid API URL: %w", err)
	}
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else if u.Scheme == "http" {
		u.Scheme = "ws"
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + connectPath
	q := u.Query()
	q.Set("instance_id", c.config.InstanceID)
	q.Set("token", c.config.Token)
	u.RawQuery = q.Encode()

	header := http.Header{}
	header.Set("Authorization", "Bearer "+c.config.Token)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}
	conn, _, err := dialer.Dial(u.String(), header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	bc := &bridgeConn{conn: conn}
	c.conn = bc

	// Send register frame (optional; API may accept query params only)
	reg := pico.NewMessage("register", map[string]any{"instance_id": c.config.InstanceID})
	reg.SessionID = c.sessionID
	if err := bc.writeJSON(reg); err != nil {
		bc.close()
		return fmt.Errorf("register: %w", err)
	}

	logger.InfoCF(channelName, "Connected to AnyClaw-API", map[string]any{
		"instance_id": c.config.InstanceID,
	})

	c.readLoop(bc)
	return nil
}

func (c *BridgeChannel) readLoop(bc *bridgeConn) {
	defer func() {
		bc.close()
		c.conn = nil
		logger.InfoCF(channelName, "Disconnected from AnyClaw-API", map[string]any{
			"instance_id": c.config.InstanceID,
		})
	}()

	_ = bc.conn.SetReadDeadline(time.Now().Add(readTimeout))
	bc.conn.SetPongHandler(func(string) error {
		_ = bc.conn.SetReadDeadline(time.Now().Add(readTimeout))
		return nil
	})

	go c.pingLoop(bc)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		_, raw, err := bc.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.DebugCF(channelName, "WebSocket read error", map[string]any{"error": err.Error()})
			}
			return
		}

		_ = bc.conn.SetReadDeadline(time.Now().Add(readTimeout))

		var msg pico.PicoMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			errMsg := pico.NewError("invalid_message", "failed to parse")
			bc.writeJSON(errMsg)
			continue
		}

		c.handleMessage(bc, msg)
	}
}

func (c *BridgeChannel) pingLoop(bc *bridgeConn) {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if bc.closed.Load() {
				return
			}
			bc.writeMu.Lock()
			_ = bc.conn.WriteMessage(websocket.PingMessage, nil)
			bc.writeMu.Unlock()
		}
	}
}

func (c *BridgeChannel) handleMessage(bc *bridgeConn, msg pico.PicoMessage) {
	switch msg.Type {
	case "registered":
		// API acknowledged registration
		logger.DebugC(channelName, "Received registered ack")
	case "pong":
		// Keepalive response
	case pico.TypeMessageSend:
		c.handleMessageSend(bc, msg)
	default:
		logger.DebugCF(channelName, "Unknown message type", map[string]any{"type": msg.Type})
	}
}

func (c *BridgeChannel) handleMessageSend(bc *bridgeConn, msg pico.PicoMessage) {
	content, _ := msg.Payload["content"].(string)
	mediaURL, _ := msg.Payload["media_url"].(string)
	mediaType, _ := msg.Payload["media_type"].(string)

	if strings.TrimSpace(content) == "" && mediaURL == "" {
		errMsg := pico.NewError("empty_content", "message content is empty")
		bc.writeJSON(errMsg)
		return
	}

	sessionID := msg.SessionID
	if sessionID == "" {
		sessionID = c.sessionID
	}

	chatID := channelName + ":" + sessionID
	senderID := "web-user"
	peer := bus.Peer{Kind: "direct", ID: chatID}

	metadata := map[string]string{
		"platform":    "anyclaw_web",
		"session_id":  sessionID,
		"instance_id": c.config.InstanceID,
	}

	sender := bus.SenderInfo{
		Platform:    "anyclaw_web",
		PlatformID:  senderID,
		CanonicalID: identity.BuildCanonicalID("anyclaw_web", senderID),
	}

	var mediaRefs []string
	if mediaURL != "" && (mediaType == "audio" || mediaType == "") {
		if store := c.GetMediaStore(); store != nil {
			filename := "voice.webm"
			if u, err := url.Parse(mediaURL); err == nil {
				base := filepath.Base(u.Path)
				if base != "" && base != "." && base != "/" {
					filename = base
				}
			}
			localPath := utils.DownloadFileSimple(mediaURL, filename)
			if localPath != "" {
				ct := "audio/webm"
				ext := strings.ToLower(filepath.Ext(filename))
				switch ext {
				case ".wav":
					ct = "audio/wav"
				case ".ogg":
					ct = "audio/ogg"
				case ".mp3":
					ct = "audio/mpeg"
				case ".m4a":
					ct = "audio/mp4"
				}
				scope := channels.BuildMediaScope(channelName, chatID, msg.ID)
				ref, err := store.Store(localPath, media.MediaMeta{
					Filename:    filename,
					ContentType: ct,
					Source:      "anyclaw_web",
				}, scope)
				if err == nil {
					mediaRefs = append(mediaRefs, ref)
					logger.DebugCF(channelName, "Stored web audio in media store", map[string]any{
						"ref": ref, "filename": filename,
					})
				} else {
					logger.WarnCF(channelName, "Failed to store web audio", map[string]any{"error": err.Error()})
				}
			}
		}
		// Always use [audio] so transcribeAudioInMessage can locate and replace it,
		// and to prevent the LLM from treating any markdown URL as a fetchable link.
		content = "[audio]"
	}

	logger.DebugCF(channelName, "Received message", map[string]any{
		"session_id":  sessionID,
		"preview":     truncate(content, 50),
		"has_media":   len(mediaRefs) > 0,
	})

	c.HandleMessage(c.ctx, peer, msg.ID, senderID, chatID, content, mediaRefs, metadata, sender)
}

// MirrorChatID returns the chat ID for mirroring outbound messages from other channels to the web.
func (c *BridgeChannel) MirrorChatID() string {
	return c.chatID
}

// PushInboundToAPI sends a user message from another channel to the API so the web UI shows it in real-time.
func (c *BridgeChannel) PushInboundToAPI(content string) error {
	if !c.IsRunning() || c.conn == nil || c.conn.closed.Load() {
		return nil
	}
	outMsg := pico.NewMessage(pico.TypeMessageCreate, map[string]any{
		"content": content,
		"role":    "user",
	})
	outMsg.SessionID = c.sessionID
	return c.conn.writeJSON(outMsg)
}

// Send implements Channel.
func (c *BridgeChannel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	if c.conn == nil || c.conn.closed.Load() {
		return fmt.Errorf("not connected to API")
	}

	sessionID := strings.TrimPrefix(msg.ChatID, channelName+":")
	if sessionID == "" {
		sessionID = c.sessionID
	}

	outMsg := pico.NewMessage(pico.TypeMessageCreate, map[string]any{
		"content": msg.Content,
		"role":    "assistant",
	})
	outMsg.SessionID = sessionID

	return c.conn.writeJSON(outMsg)
}

// EditMessage implements channels.MessageEditor.
func (c *BridgeChannel) EditMessage(ctx context.Context, chatID string, messageID string, content string) error {
	if c.conn == nil || c.conn.closed.Load() {
		return fmt.Errorf("not connected to API")
	}
	sessionID := strings.TrimPrefix(chatID, channelName+":")
	if sessionID == "" {
		sessionID = c.sessionID
	}
	outMsg := pico.NewMessage(pico.TypeMessageUpdate, map[string]any{
		"message_id": messageID,
		"content":    content,
		"role":       "assistant",
	})
	outMsg.SessionID = sessionID
	return c.conn.writeJSON(outMsg)
}

// StartTyping implements channels.TypingCapable.
func (c *BridgeChannel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	if c.conn == nil || c.conn.closed.Load() {
		return func() {}, nil
	}
	startMsg := pico.NewMessage(pico.TypeTypingStart, nil)
	startMsg.SessionID = c.sessionID
	if err := c.conn.writeJSON(startMsg); err != nil {
		return func() {}, err
	}
	return func() {
		stopMsg := pico.NewMessage(pico.TypeTypingStop, nil)
		stopMsg.SessionID = c.sessionID
		c.conn.writeJSON(stopMsg)
	}, nil
}

// SendPlaceholder implements channels.PlaceholderCapable.
func (c *BridgeChannel) SendPlaceholder(ctx context.Context, chatID string) (string, error) {
	if c.conn == nil || c.conn.closed.Load() {
		return "", nil
	}
	msgID := uuid.New().String()
	outMsg := pico.NewMessage(pico.TypeMessageCreate, map[string]any{
		"content":     "Thinking... 💭",
		"message_id": msgID,
	})
	outMsg.SessionID = c.sessionID
	if err := c.conn.writeJSON(outMsg); err != nil {
		return "", err
	}
	return msgID, nil
}

// compressImageForWeb compresses images for web transmission: re-encode as JPEG (quality 82).
// For large images (>1920x1080), scales down to reduce payload. Returns compressed data and new content type.
func compressImageForWeb(data []byte, contentType string) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", err
	}
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	const maxW, maxH = 1920, 1080
	var target image.Image = img
	if w > maxW || h > maxH {
		scale := float64(maxW) / float64(w)
		if float64(h)*scale > float64(maxH) {
			scale = float64(maxH) / float64(h)
		}
		nw := int(float64(w)*scale + 0.5)
		nh := int(float64(h)*scale + 0.5)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		for y := 0; y < nh; y++ {
			for x := 0; x < nw; x++ {
				sx := bounds.Min.X + int(float64(x)/scale+0.5)
				sy := bounds.Min.Y + int(float64(y)/scale+0.5)
				if sx >= bounds.Max.X {
					sx = bounds.Max.X - 1
				}
				if sy >= bounds.Max.Y {
					sy = bounds.Max.Y - 1
				}
				dst.Set(x, y, img.At(sx, sy))
			}
		}
		target = dst
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, target, &jpeg.Options{Quality: 82}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

// uploadToAPI 上传文件到 API（会转发到 COS），返回 URL；失败返回空字符串
func (c *BridgeChannel) uploadToAPI(ctx context.Context, filePath, filename, contentType string, data []byte) string {
	baseURL := strings.TrimSuffix(c.config.APIURL, "/")
	uploadURL := baseURL + "/instances/" + c.config.InstanceID + "/media"
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return ""
	}
	if _, err := part.Write(data); err != nil {
		return ""
	}
	if err := w.Close(); err != nil {
		return ""
	}
	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, &buf)
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.config.Token)
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logger.DebugCF(channelName, "Upload to API failed", map[string]any{"error": err.Error()})
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil || out.URL == "" {
		return ""
	}
	return out.URL
}

// sanitizeFilenameForMarkdown 移除会破坏 markdown 链接语法的字符
func sanitizeFilenameForMarkdown(s string) string {
	return strings.NewReplacer("[", "-", "]", "-", "(", "-", ")", "-", "\\", "-").Replace(s)
}

// inferMediaTypeFromMeta infers "image"|"audio"|"video"|"file" from filename and content type.
func inferMediaTypeFromMeta(filename, contentType string) string {
	ct := strings.ToLower(contentType)
	fn := strings.ToLower(filename)
	if strings.HasPrefix(ct, "image/") {
		return "image"
	}
	if strings.HasPrefix(ct, "audio/") || ct == "application/ogg" {
		return "audio"
	}
	if strings.HasPrefix(ct, "video/") {
		return "video"
	}
	ext := filepath.Ext(fn)
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".svg":
		return "image"
	case ".mp3", ".wav", ".ogg", ".m4a", ".flac", ".aac", ".wma", ".opus":
		return "audio"
	case ".mp4", ".avi", ".mov", ".webm", ".mkv":
		return "video"
	}
	return "file"
}

// SendMedia implements channels.MediaSender. Tries to upload to COS via API first;
// falls back to base64 embedding when COS is not configured.
func (c *BridgeChannel) SendMedia(ctx context.Context, msg bus.OutboundMediaMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	if c.conn == nil || c.conn.closed.Load() {
		return fmt.Errorf("not connected to API")
	}
	store := c.GetMediaStore()
	if store == nil {
		return fmt.Errorf("media store not configured")
	}

	sessionID := strings.TrimPrefix(msg.ChatID, channelName+":")
	if sessionID == "" {
		sessionID = c.sessionID
	}

	const maxEmbedSize = 4 << 20 // 4MB per file (fallback base64)
	var parts []string
	for _, p := range msg.Parts {
		path, meta, err := store.ResolveWithMeta(p.Ref)
		if err != nil {
			logger.WarnCF(channelName, "Failed to resolve media ref", map[string]any{
				"ref": p.Ref, "error": err.Error(),
			})
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			logger.WarnCF(channelName, "Failed to read media file", map[string]any{
				"path": path, "error": err.Error(),
			})
			continue
		}
		ct := meta.ContentType
		if ct == "" {
			ct = mime.TypeByExtension(filepath.Ext(meta.Filename))
		}
		if ct == "" {
			ct = "application/octet-stream"
		}
		mediaType := p.Type
		if mediaType == "" {
			mediaType = inferMediaTypeFromMeta(meta.Filename, ct)
		}
		if mediaType == "image" {
			if compressed, newCT, err := compressImageForWeb(data, ct); err == nil {
				data = compressed
				ct = newCT
			}
		}
		linkPrefix := "📎"
		switch mediaType {
		case "video":
			linkPrefix = "📹"
		case "audio":
			linkPrefix = "🔊"
		}
		filename := meta.Filename
		if filename == "" {
			filename = p.Filename
		}
		if filename == "" {
			filename = filepath.Base(path)
		}
		if filename == "" {
			filename = "file"
		}
		filename = sanitizeFilenameForMarkdown(filename)
		// 优先上传到 COS
		fileURL := c.uploadToAPI(ctx, path, filename, ct, data)
		if fileURL != "" {
			if mediaType == "image" {
				parts = append(parts, fmt.Sprintf("![%s](%s)", filename, fileURL))
			} else {
				parts = append(parts, fmt.Sprintf("[%s %s](%s)", linkPrefix, filename, fileURL))
			}
			continue
		}
		// 回退：base64 嵌入（仅小文件）
		if len(data) > maxEmbedSize {
			parts = append(parts, fmt.Sprintf("%s %s（文件过大，未嵌入）", linkPrefix, filename))
			continue
		}
		b64 := base64.StdEncoding.EncodeToString(data)
		dataURL := fmt.Sprintf("data:%s;base64,%s", ct, b64)
		if mediaType == "image" {
			parts = append(parts, fmt.Sprintf("![%s](%s)", filename, dataURL))
		} else {
			parts = append(parts, fmt.Sprintf("[%s %s](%s)", linkPrefix, filename, dataURL))
		}
	}
	if len(parts) == 0 {
		return nil
	}
	content := strings.Join(parts, "\n\n")
	outMsg := pico.NewMessage(pico.TypeMessageCreate, map[string]any{
		"content": content,
		"role":    "assistant",
	})
	outMsg.SessionID = sessionID
	return c.conn.writeJSON(outMsg)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
