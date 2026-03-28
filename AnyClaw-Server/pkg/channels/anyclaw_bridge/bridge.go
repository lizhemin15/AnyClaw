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
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/fileutil"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/identity"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/routing"
	"github.com/anyclaw/anyclaw-server/pkg/tools"
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
	instanceMailPollInterval = 90 * time.Second
	instanceMailFetchAttempts = 3
)

type instanceMailWatermarkFile struct {
	MaxID int64 `json:"max_id"`
}

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
	config       config.AnyClawBridgeConfig
	workspacePath string
	rosterSlugs  []string
	conn         *bridgeConn
	ctx          context.Context
	cancel       context.CancelFunc
	chatID       string
	sessionID    string
	collabClient *tools.CollabAPIClient

	instanceMailMu             sync.Mutex
	instanceMailWatermark      int64 // persisted max id; baseline + hint
	instanceMailBaselineDone    atomic.Bool
	instanceMailNeedsWarmup     atomic.Bool
	instanceMailDelivered       sync.Map // int64 msg id -> struct{} (dedup WS vs poll)
}

func agentSlugsFromAgentConfig(cfg *config.Config) []string {
	if cfg == nil {
		return []string{"main"}
	}
	if len(cfg.Agents.List) == 0 {
		return []string{"main"}
	}
	ids := make([]string, 0, len(cfg.Agents.List))
	for i := range cfg.Agents.List {
		ids = append(ids, routing.NormalizeAgentID(cfg.Agents.List[i].ID))
	}
	sort.Strings(ids)
	return ids
}

// NewBridgeChannel creates an outbound bridge channel.
func NewBridgeChannel(cfg *config.Config, messageBus *bus.MessageBus) (*BridgeChannel, error) {
	br := cfg.Channels.AnyClawBridge
	if !br.IsEnabled() {
		return nil, fmt.Errorf("anyclaw_bridge requires ANYCLAW_API_URL, ANYCLAW_INSTANCE_ID, ANYCLAW_TOKEN")
	}

	base := channels.NewBaseChannel(channelName, br, messageBus, nil)
	chatID := channelName + ":" + br.InstanceID
	bc := &BridgeChannel{
		BaseChannel:   base,
		config:        br,
		workspacePath: cfg.WorkspacePath(),
		rosterSlugs:   agentSlugsFromAgentConfig(cfg),
		chatID:        chatID,
		sessionID:     br.InstanceID,
		collabClient:  tools.NewCollabAPIClient(br.APIURL, br.InstanceID, br.Token),
	}
	bc.loadInstanceMailWatermark()
	if bc.instanceMailWatermark > 0 {
		bc.instanceMailBaselineDone.Store(true)
		bc.instanceMailNeedsWarmup.Store(true)
	}
	return bc, nil
}

func (c *BridgeChannel) instanceMailWatermarkPath() string {
	return filepath.Join(c.workspacePath, ".anyclaw", "instance_mail_watermark.json")
}

func (c *BridgeChannel) loadInstanceMailWatermark() {
	p := c.instanceMailWatermarkPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	var f instanceMailWatermarkFile
	if json.Unmarshal(data, &f) != nil || f.MaxID < 0 {
		return
	}
	c.instanceMailWatermark = f.MaxID
}

func (c *BridgeChannel) saveInstanceMailWatermarkLocked() {
	dir := filepath.Dir(c.instanceMailWatermarkPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(instanceMailWatermarkFile{MaxID: c.instanceMailWatermark})
	if err != nil {
		return
	}
	_ = fileutil.WriteFileAtomic(c.instanceMailWatermarkPath(), b, 0o644)
}

func (c *BridgeChannel) syncRosterOnce() {
	if c.collabClient == nil || len(c.rosterSlugs) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	ids := append([]string(nil), c.rosterSlugs...)
	added, err := c.collabClient.SyncRosterSlugs(ctx, ids)
	if err != nil {
		logger.WarnCF(channelName, "协作员工与容器 agents.list 同步失败（可稍后在网页编排中查看）",
			map[string]any{"error": err.Error()})
	} else if added > 0 {
		logger.InfoCF(channelName, "已自动追加协作员工记录",
			map[string]any{"added": added, "slugs": ids})
	}
}

// Start implements Channel. Connects outbound to API and starts read loop.
func (c *BridgeChannel) Start(ctx context.Context) error {
	logger.InfoC(channelName, "Starting AnyClaw outbound bridge")
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.SetRunning(true)

	go c.connectLoop()
	go c.instanceMailPollLoop()
	logger.InfoC(channelName, "AnyClaw outbound bridge started")
	return nil
}

func (c *BridgeChannel) instanceMailPollLoop() {
	if c.collabClient == nil {
		return
	}
	time.Sleep(8 * time.Second)
	c.pollInstanceMailOnce()
	ticker := time.NewTicker(instanceMailPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.pollInstanceMailOnce()
		}
	}
}

func (c *BridgeChannel) pollInstanceMailOnce() {
	if c.collabClient == nil {
		return
	}
	myID, err := strconv.ParseInt(strings.TrimSpace(c.config.InstanceID), 10, 64)
	if err != nil || myID < 1 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	out, err := c.collabClient.GetInstanceMessageList(ctx, 100, 0, nil)
	if err != nil {
		logger.WarnCF(channelName, "instance mail poll: list failed",
			map[string]any{"error": err.Error()})
		return
	}
	raw, ok := out["messages"].([]any)
	if !ok {
		return
	}
	var rows []map[string]any
	for _, m := range raw {
		row, ok := m.(map[string]any)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		idi, ok1 := tools.CollabMailIDFromNotify(rows[i])
		idj, ok2 := tools.CollabMailIDFromNotify(rows[j])
		if !ok1 {
			return true
		}
		if !ok2 {
			return false
		}
		return idi < idj
	})

	if c.instanceMailNeedsWarmup.Load() {
		c.instanceMailMu.Lock()
		wm := c.instanceMailWatermark
		c.instanceMailMu.Unlock()
		for _, row := range rows {
			toID, ok2 := collabPeerInstanceID(row, "to_instance_id")
			if !ok2 || toID != myID {
				continue
			}
			id, ok := tools.CollabMailIDFromNotify(row)
			if !ok || id < 1 || id > wm {
				continue
			}
			c.instanceMailDelivered.Store(id, true)
		}
		c.instanceMailNeedsWarmup.Store(false)
	}

	if !c.instanceMailBaselineDone.Load() {
		var maxID int64
		if c.instanceMailWatermark > maxID {
			maxID = c.instanceMailWatermark
		}
		for _, row := range rows {
			toID, ok2 := collabPeerInstanceID(row, "to_instance_id")
			if !ok2 || toID != myID {
				continue
			}
			id, ok := tools.CollabMailIDFromNotify(row)
			if ok && id > 0 {
				c.instanceMailDelivered.Store(id, true)
				if id > maxID {
					maxID = id
				}
			}
		}
		c.instanceMailMu.Lock()
		c.instanceMailWatermark = maxID
		c.saveInstanceMailWatermarkLocked()
		c.instanceMailMu.Unlock()
		c.instanceMailBaselineDone.Store(true)
		logger.DebugCF(channelName, "instance mail poll: baseline (mark seen, no agent turn)",
			map[string]any{"max_id": maxID})
		return
	}

	for _, row := range rows {
		toID, ok2 := collabPeerInstanceID(row, "to_instance_id")
		if !ok2 || toID != myID {
			continue
		}
		msgID, ok := tools.CollabMailIDFromNotify(row)
		if !ok || msgID < 1 {
			continue
		}
		content := collabMapString(row, "content")
		if strings.TrimSpace(content) == "" {
			continue
		}
		fromID, ok1 := collabPeerInstanceID(row, "from_instance_id")
		if !ok1 || fromID < 1 {
			continue
		}
		c.deliverInstanceMailInbound(msgID, fromID, myID, content, collabMapString(row, "created_at"))
	}
}

func (c *BridgeChannel) fetchInstanceMessageWithRetry(ctx context.Context, msgID int64, fromID int64) (map[string]any, error) {
	var lastErr error
	for attempt := 0; attempt < instanceMailFetchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(300*(1<<attempt)) * time.Millisecond):
			}
		}
		row, err := c.collabClient.GetInstanceMessageByID(ctx, msgID, fromID)
		if err == nil {
			return row, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (c *BridgeChannel) bumpInstanceMailWatermark(msgID int64) {
	c.instanceMailMu.Lock()
	defer c.instanceMailMu.Unlock()
	if msgID > c.instanceMailWatermark {
		c.instanceMailWatermark = msgID
		c.saveInstanceMailWatermarkLocked()
	}
}

func (c *BridgeChannel) publishInstanceMailInbound(msgID, fromID, toID int64, content, createdAt string) error {
	agentSlug := routing.NormalizeAgentID(routing.DefaultAgentID)
	sessionKey := fmt.Sprintf("agent:%s:instance_mail:%d", agentSlug, fromID)
	chatID := fmt.Sprintf("instance_mail:%d", fromID)
	meta := map[string]string{
		"from_instance_id": fmt.Sprintf("%d", fromID),
		"to_instance_id":   fmt.Sprintf("%d", toID),
		"msg_id":           fmt.Sprintf("%d", msgID),
	}
	if createdAt != "" {
		meta["created_at"] = createdAt
	}
	userText := fmt.Sprintf(
		"[跨实例消息 id=%d]\nFrom instance: %d\nTo instance: %d\n\n%s",
		msgID, fromID, toID, content,
	)
	in := bus.InboundMessage{
		Channel:    "instance_mail",
		SenderID:   fmt.Sprintf("instance:%d", fromID),
		ChatID:     chatID,
		Content:    userText,
		Metadata:   meta,
		SessionKey: sessionKey,
		Peer:       bus.Peer{Kind: "direct", ID: chatID},
		MessageID:  fmt.Sprintf("imsg-%d", msgID),
	}
	pubCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := c.MessageBus().PublishInbound(pubCtx, in); err != nil {
		return err
	}
	return nil
}

func (c *BridgeChannel) deliverInstanceMailInbound(msgID, fromID, toID int64, content, createdAt string) {
	if _, loaded := c.instanceMailDelivered.LoadOrStore(msgID, true); loaded {
		return
	}
	if err := c.publishInstanceMailInbound(msgID, fromID, toID, content, createdAt); err != nil {
		c.instanceMailDelivered.Delete(msgID)
		logger.WarnCF(channelName, "publish instance_mail inbound failed", map[string]any{"error": err.Error()})
		return
	}
	c.bumpInstanceMailWatermark(msgID)
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

	go c.syncRosterOnce()

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
	case "collab.topology_updated":
		c.handleCollabTopologyUpdated(msg)
	case "collab.internal_mail":
		c.handleCollabInternalMail(msg)
	case "collab.instance_mail":
		c.handleCollabInstanceMail(msg)
	default:
		logger.DebugCF(channelName, "Unknown message type", map[string]any{"type": msg.Type})
	}
}

func (c *BridgeChannel) handleCollabTopologyUpdated(msg pico.PicoMessage) {
	var ver any
	if msg.Payload != nil {
		ver = msg.Payload["version"]
	}
	logger.InfoCF(channelName, "Collab topology updated (reload roster/topology via tools if needed)",
		map[string]any{"version": ver})
}

func (c *BridgeChannel) handleCollabInternalMail(msg pico.PicoMessage) {
	if c.collabClient == nil || msg.Payload == nil {
		return
	}
	mailID, ok := tools.CollabMailIDFromNotify(msg.Payload)
	if !ok || mailID < 1 {
		logger.WarnCF(channelName, "collab.internal_mail missing id", nil)
		return
	}
	row, err := c.collabClient.GetInternalMail(c.ctx, mailID)
	if err != nil {
		logger.WarnCF(channelName, "fetch internal mail failed", map[string]any{"id": mailID, "error": err.Error()})
		return
	}
	toSlug := collabMapString(row, "to_slug")
	fromSlug := collabMapString(row, "from_slug")
	threadID := collabMapString(row, "thread_id")
	subject := collabMapString(row, "subject")
	body := collabMapString(row, "body")
	if toSlug == "" || threadID == "" {
		logger.WarnCF(channelName, "internal mail row incomplete", map[string]any{"id": mailID})
		return
	}
	norm := routing.NormalizeAgentID(toSlug)
	sessionKey := fmt.Sprintf("agent:%s:internal_mail:%s", norm, threadID)
	chatID := "internal_mail:" + threadID
	meta := map[string]string{
		"to_slug":    toSlug,
		"from_slug":  fromSlug,
		"thread_id":  threadID,
		"mail_id":    fmt.Sprintf("%d", mailID),
		"subject":    subject,
	}
	if tv := row["topology_version"]; tv != nil {
		meta["topology_version"] = fmt.Sprint(tv)
	}
	userText := fmt.Sprintf(
		"[Internal mail id=%d]\nFrom: %s\nTo: %s\nThread: %s\nSubject: %s\n\n%s",
		mailID, fromSlug, toSlug, threadID, subject, body,
	)
	in := bus.InboundMessage{
		Channel:    "internal_mail",
		SenderID:   "internal_mail:" + fromSlug,
		ChatID:     chatID,
		Content:    userText,
		Metadata:   meta,
		SessionKey: sessionKey,
		Peer:       bus.Peer{Kind: "direct", ID: chatID},
		MessageID:  fmt.Sprintf("im-%d", mailID),
	}
	if err := c.MessageBus().PublishInbound(c.ctx, in); err != nil {
		logger.WarnCF(channelName, "publish internal_mail inbound failed", map[string]any{"error": err.Error()})
	}
}

// handleCollabInstanceMail handles API push for cross-instance messages (recipient container only).
func (c *BridgeChannel) handleCollabInstanceMail(msg pico.PicoMessage) {
	if c.collabClient == nil || msg.Payload == nil {
		return
	}
	msgID, ok := tools.CollabMailIDFromNotify(msg.Payload)
	if !ok || msgID < 1 {
		logger.WarnCF(channelName, "collab.instance_mail missing id", nil)
		return
	}
	fromID, ok1 := collabPeerInstanceID(msg.Payload, "from_instance_id")
	toID, ok2 := collabPeerInstanceID(msg.Payload, "to_instance_id")
	if !ok1 || !ok2 || fromID < 1 || toID < 1 {
		logger.WarnCF(channelName, "collab.instance_mail missing from/to instance", nil)
		return
	}
	myID, err := strconv.ParseInt(strings.TrimSpace(c.config.InstanceID), 10, 64)
	if err != nil || myID < 1 {
		return
	}
	if myID != toID {
		// Sender's container also receives the same WS frame; only the recipient should inject inbound.
		return
	}
	if _, loaded := c.instanceMailDelivered.LoadOrStore(msgID, true); loaded {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	row, err := c.fetchInstanceMessageWithRetry(ctx, msgID, fromID)
	if err != nil {
		c.instanceMailDelivered.Delete(msgID)
		logger.WarnCF(channelName, "fetch instance message failed", map[string]any{"id": msgID, "error": err.Error()})
		return
	}
	content := collabMapString(row, "content")
	if strings.TrimSpace(content) == "" {
		c.instanceMailDelivered.Delete(msgID)
		logger.WarnCF(channelName, "instance message row incomplete", map[string]any{"id": msgID})
		return
	}
	if err := c.publishInstanceMailInbound(msgID, fromID, toID, content, collabMapString(row, "created_at")); err != nil {
		c.instanceMailDelivered.Delete(msgID)
		logger.WarnCF(channelName, "publish instance_mail inbound failed", map[string]any{"error": err.Error()})
		return
	}
	c.bumpInstanceMailWatermark(msgID)
}

func collabPeerInstanceID(payload map[string]any, key string) (int64, bool) {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

func collabMapString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	default:
		return fmt.Sprint(x)
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
	mt := strings.ToLower(strings.TrimSpace(mediaType))
	httpDL := &http.Client{Timeout: 2 * time.Minute}

	if mediaURL != "" {
		if store := c.GetMediaStore(); store != nil {
			switch {
			case mt == "image" || strings.HasPrefix(mt, "image/"):
				fn := "image.jpg"
				if u, err := url.Parse(mediaURL); err == nil {
					base := filepath.Base(u.Path)
					if base != "" && base != "." && base != "/" {
						fn = base
					}
				}
				scope := channels.BuildMediaScope(channelName, chatID, msg.ID)
				ref, err := channels.DownloadHTTPURLToMediaStore(c.ctx, httpDL, store, mediaURL, scope, fn, mt, "anyclaw_web")
				if err != nil {
					logger.WarnCF(channelName, "Failed to download inbound image", map[string]any{"error": err.Error()})
				} else {
					mediaRefs = append(mediaRefs, ref)
				}
			case mt == "audio" || mt == "":
				filename := "voice.webm"
				if u, err := url.Parse(mediaURL); err == nil {
					base := filepath.Base(u.Path)
					if base != "" && base != "." && base != "/" {
						filename = base
					}
				}
				localPath := utils.DownloadFileSimple(mediaURL, filename)
				if localPath == "" {
					logger.WarnCF(channelName, "Failed to download inbound audio, transcription will be skipped. Ensure COS bucket/objects are publicly readable.", map[string]any{
						"media_url": mediaURL,
					})
				}
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
				content = "[audio]"
			default:
				logger.WarnCF(channelName, "Unknown inbound media_type", map[string]any{"media_type": mediaType})
			}
		}
		if mt == "image" || strings.HasPrefix(mt, "image/") {
			content = channels.AppendImageMediaPlaceholder(content)
		}
	}

	if strings.TrimSpace(content) == "" && len(mediaRefs) == 0 && mediaURL != "" {
		errMsg := pico.NewError("bad_media", "unsupported media_type or media could not be processed")
		bc.writeJSON(errMsg)
		return
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
