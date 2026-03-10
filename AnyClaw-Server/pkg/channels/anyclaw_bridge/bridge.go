package anyclaw_bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	if strings.TrimSpace(content) == "" {
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

	logger.DebugCF(channelName, "Received message", map[string]any{
		"session_id": sessionID,
		"preview":    truncate(content, 50),
	})

	c.HandleMessage(c.ctx, peer, msg.ID, senderID, chatID, content, nil, metadata, sender)
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

	outMsg := pico.NewMessage(pico.TypeMessageCreate, map[string]any{"content": msg.Content})
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

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
