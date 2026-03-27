// Package weixinclaw implements the WeChat ClawBot (ilink) channel natively in AnyClaw,
// using the same credential layout as @tencent-weixin/openclaw-weixin (openclaw-weixin/ under state root).
package weixinclaw

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/bus"
	"github.com/anyclaw/anyclaw-server/pkg/channels"
	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/identity"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
)

func init() {
	channels.RegisterFactory("weixinclaw", NewChannel)
}

const (
	defaultBaseURL = "https://ilinkai.weixin.qq.com"
	longPollMS     = 35_000 // same default window as openclaw-weixin api.ts DEFAULT_LONG_POLL_TIMEOUT_MS
	// weixinOpenClawPluginNPM is the @tencent-weixin/openclaw-weixin version we track for ilink payloads.
	weixinOpenClawPluginNPM = "1.0.3"
	apiTimeout              = 15 * time.Second
	sessionExpiredCode      = -14
	maxFailures             = 3
	backoffOnFail           = 30 * time.Second
	retryDelay              = 2 * time.Second
)

func weixinBaseInfo() map[string]string {
	return map[string]string{"channel_version": "anyclaw-weixinclaw/" + weixinOpenClawPluginNPM}
}

// Channel long-polls ilink getUpdates and sends replies via sendmessage (text + media).
type Channel struct {
	*channels.BaseChannel
	cfg       *config.Config
	stateRoot string
	routeTag  string
	cdnBase   string
	accs      []*weixinAccount

	mu     sync.RWMutex
	ctxTok map[string]string // key: accountID \x00 peerUserID -> context_token
	seen   map[string]struct{}

	runCtx    context.Context
	runCancel context.CancelFunc
	wg        sync.WaitGroup
}

type weixinAccount struct {
	ID      string
	Token   string
	BaseURL string
}

func NewChannel(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
	wcfg := cfg.Channels.WeixinClaw
	root, err := resolveStateRoot(strings.TrimSpace(wcfg.StateDir))
	if err != nil {
		return nil, fmt.Errorf("weixinclaw: state dir: %w", err)
	}
	if err := tryMigrateWeixinDiskIntoConfig(cfg, root); err != nil {
		logger.WarnCF("weixinclaw", "weixin: 将磁盘凭证合并进 config.json 失败（仍尝试从磁盘读取）", map[string]any{"error": err.Error()})
	}
	accs, err := mergeLoadWeixinAccounts(cfg, root)
	if err != nil {
		return nil, fmt.Errorf("weixinclaw: load accounts: %w", err)
	}
	if len(accs) == 0 {
		logger.WarnCF("weixinclaw", "No weixin token in config.json (channels.weixin_claw.accounts) or openclaw-weixin/ — 完成「绑定微信」后重启网关", map[string]any{"state_root": root})
	}

	base := channels.NewBaseChannel(
		"weixinclaw",
		wcfg,
		b,
		wcfg.AllowFrom,
		channels.WithMaxMessageLength(4000),
		channels.WithReasoningChannelID(wcfg.ReasoningChannelID),
	)
	ch := &Channel{
		BaseChannel: base,
		cfg:         cfg,
		stateRoot:   root,
		routeTag:    strings.TrimSpace(wcfg.RouteTag),
		cdnBase:     strings.TrimSpace(wcfg.CdnBaseURL),
		accs:        accs,
		ctxTok:      make(map[string]string),
		seen:        make(map[string]struct{}),
	}
	return ch, nil
}

func resolveStateRoot(override string) (string, error) {
	if override != "" {
		return filepath.Abs(expandHome(override))
	}
	if p := strings.TrimSpace(os.Getenv("ANYCLAW_CONFIG")); p != "" {
		dir := filepath.Dir(p)
		return filepath.Abs(expandHome(dir))
	}
	if d := strings.TrimSpace(os.Getenv("OPENCLAW_STATE_DIR")); d != "" {
		return filepath.Abs(expandHome(d))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if h := strings.TrimSpace(os.Getenv("OPENCLAW_HOME")); h != "" {
		return filepath.Abs(expandHome(h))
	}
	openclaw := filepath.Join(home, ".openclaw")
	if st, err := os.Stat(openclaw); err == nil && st.IsDir() {
		return filepath.Abs(openclaw)
	}
	if h := strings.TrimSpace(os.Getenv("ANYCLAW_HOME")); h != "" {
		return filepath.Abs(expandHome(h))
	}
	return filepath.Abs(filepath.Join(home, ".anyclaw"))
}

func expandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, _ := os.UserHomeDir()
	if len(p) > 1 && p[1] == '/' {
		return filepath.Join(home, p[2:])
	}
	return home
}

type accountFile struct {
	Token   string `json:"token"`
	BaseURL string `json:"baseUrl"`
}

// deriveRawAccountID reverses openclaw-weixin normalizeAccountId for legacy filenames (accounts.ts).
func deriveRawAccountID(normalizedID string) string {
	n := strings.TrimSpace(normalizedID)
	if strings.HasSuffix(n, "-im-bot") && len(n) > len("-im-bot") {
		return n[:len(n)-len("-im-bot")] + "@im.bot"
	}
	if strings.HasSuffix(n, "-im-wechat") && len(n) > len("-im-wechat") {
		return n[:len(n)-len("-im-wechat")] + "@im.wechat"
	}
	return ""
}

// tryMigrateWeixinDiskIntoConfig 在 config 里尚无 accounts、但磁盘上已有 openclaw-weixin/ 时，把凭证追加写入 config.json（不删其它字段；不新建不存在的 config 文件）。
func tryMigrateWeixinDiskIntoConfig(cfg *config.Config, stateRoot string) error {
	if len(cfg.Channels.WeixinClaw.Accounts) > 0 {
		return nil
	}
	disk, err := loadWeixinAccountsFromDisk(stateRoot)
	if err != nil {
		return err
	}
	if len(disk) == 0 {
		return nil
	}
	path := config.DefaultConfigPath()
	if st, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	} else if st.IsDir() {
		return fmt.Errorf("ANYCLAW_CONFIG is a directory, expected config.json path")
	}
	onDisk, err := config.LoadConfig(path)
	if err != nil {
		return err
	}
	if len(onDisk.Channels.WeixinClaw.Accounts) > 0 {
		cfg.Channels.WeixinClaw.Accounts = onDisk.Channels.WeixinClaw.Accounts
		return nil
	}
	for _, a := range disk {
		if a == nil || strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Token) == "" {
			continue
		}
		base := strings.TrimSpace(a.BaseURL)
		if base == "" {
			base = defaultBaseURL
		}
		onDisk.Channels.WeixinClaw.Accounts = append(onDisk.Channels.WeixinClaw.Accounts, config.WeixinClawAccount{
			AccountID: a.ID,
			Token:     a.Token,
			BaseURL:   base,
		})
	}
	if len(onDisk.Channels.WeixinClaw.Accounts) == 0 {
		return nil
	}
	if err := config.SaveConfig(path, onDisk); err != nil {
		return err
	}
	cfg.Channels.WeixinClaw.Accounts = onDisk.Channels.WeixinClaw.Accounts
	logger.InfoCF("weixinclaw", "已把既有 openclaw-weixin/ 凭证追加写入 config.json（与飞书同级持久化）", map[string]any{"accounts": len(onDisk.Channels.WeixinClaw.Accounts)})
	return nil
}

// mergeLoadWeixinAccounts 合并 config.json 中的 channels.weixin_claw.accounts 与磁盘 openclaw-weixin/；同 ID 以 config 为准。
func mergeLoadWeixinAccounts(cfg *config.Config, stateRoot string) ([]*weixinAccount, error) {
	disk, err := loadWeixinAccountsFromDisk(stateRoot)
	if err != nil {
		return nil, err
	}
	var fromCfg []*weixinAccount
	for _, a := range cfg.Channels.WeixinClaw.Accounts {
		id := strings.TrimSpace(a.AccountID)
		tok := strings.TrimSpace(a.Token)
		if id == "" || tok == "" {
			continue
		}
		base := strings.TrimSpace(a.BaseURL)
		if base == "" {
			base = defaultBaseURL
		}
		fromCfg = append(fromCfg, &weixinAccount{ID: id, Token: tok, BaseURL: base})
	}
	if len(fromCfg) == 0 {
		return disk, nil
	}
	byID := make(map[string]*weixinAccount)
	for _, a := range disk {
		if a != nil {
			byID[a.ID] = a
		}
	}
	for _, a := range fromCfg {
		byID[a.ID] = a
	}
	out := make([]*weixinAccount, 0, len(byID))
	for _, a := range byID {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func loadWeixinAccountsFromDisk(stateRoot string) ([]*weixinAccount, error) {
	idxPath := filepath.Join(stateRoot, "openclaw-weixin", "accounts.json")
	raw, err := os.ReadFile(idxPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	if err := json.Unmarshal(raw, &ids); err != nil {
		return nil, err
	}
	var out []*weixinAccount
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		p := filepath.Join(stateRoot, "openclaw-weixin", "accounts", id+".json")
		b, err := os.ReadFile(p)
		if err != nil && deriveRawAccountID(id) != "" {
			p2 := filepath.Join(stateRoot, "openclaw-weixin", "accounts", deriveRawAccountID(id)+".json")
			b, err = os.ReadFile(p2)
		}
		if err != nil {
			continue
		}
		var af accountFile
		if err := json.Unmarshal(b, &af); err != nil {
			continue
		}
		tok := strings.TrimSpace(af.Token)
		if tok == "" {
			continue
		}
		base := strings.TrimSpace(af.BaseURL)
		if base == "" {
			base = defaultBaseURL
		}
		out = append(out, &weixinAccount{ID: id, Token: tok, BaseURL: base})
	}
	if len(out) == 0 {
		if tok, bu := loadLegacyWeixinCredential(stateRoot); tok != "" {
			base := strings.TrimSpace(bu)
			if base == "" {
				base = defaultBaseURL
			}
			logger.InfoCF("weixinclaw", "Using legacy credentials/openclaw-weixin/credentials.json (no accounts.json)", map[string]any{"state_root": stateRoot})
			out = append(out, &weixinAccount{ID: "default", Token: tok, BaseURL: base})
		}
	}
	return out, nil
}

// loadLegacyWeixinCredential reads credentials/openclaw-weixin/credentials.json (openclaw-weixin accounts.ts).
func loadLegacyWeixinCredential(stateRoot string) (token, baseURL string) {
	p := filepath.Join(stateRoot, "credentials", "openclaw-weixin", "credentials.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		return "", ""
	}
	var v struct {
		Token   string `json:"token"`
		BaseURL string `json:"baseUrl"`
	}
	if json.Unmarshal(raw, &v) != nil {
		return "", ""
	}
	return strings.TrimSpace(v.Token), strings.TrimSpace(v.BaseURL)
}

func (c *Channel) Name() string { return "weixinclaw" }

func (c *Channel) Start(ctx context.Context) error {
	c.runCtx, c.runCancel = context.WithCancel(ctx)
	c.SetRunning(true)
	if len(c.accs) == 0 {
		logger.InfoC("weixinclaw", "Skipping long-poll (no accounts)")
		return nil
	}
	for _, acc := range c.accs {
		c.loadContextTokensForAccount(acc.ID)
	}
	for _, acc := range c.accs {
		a := acc
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.pollLoop(c.runCtx, a)
		}()
	}
	logger.InfoCF("weixinclaw", "Started ilink long-poll", map[string]any{"accounts": len(c.accs)})
	return nil
}

func (c *Channel) Stop(ctx context.Context) error {
	if c.runCancel != nil {
		c.runCancel()
	}
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
	c.SetRunning(false)
	logger.InfoC("weixinclaw", "Stopped")
	return nil
}

func (c *Channel) Send(ctx context.Context, msg bus.OutboundMessage) error {
	if !c.IsRunning() {
		return channels.ErrNotRunning
	}
	acc := c.pickAccountForPeer(msg.ChatID)
	if acc == nil {
		return fmt.Errorf("%w: no weixin account", channels.ErrSendFailed)
	}
	ctxTok := c.getCtxTok(acc.ID, msg.ChatID)
	body := buildSendBody(msg.ChatID, stripMarkdownLite(msg.Content), ctxTok)
	raw, err := c.postJSON(ctx, acc, "ilink/bot/sendmessage", body, apiTimeout)
	if err != nil {
		logger.ErrorCF("weixinclaw", "send failed", map[string]any{"error": err.Error(), "to": msg.ChatID})
		return channels.ErrTemporary
	}
	if err := weixinJSONBizError(raw); err != nil {
		logger.ErrorCF("weixinclaw", "sendmessage biz error", map[string]any{"error": err.Error(), "to": msg.ChatID})
		return channels.ErrTemporary
	}
	return nil
}

func (c *Channel) pickAccountForPeer(peer string) *weixinAccount {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.accs) == 1 {
		return c.accs[0]
	}
	for _, acc := range c.accs {
		if _, ok := c.ctxTok[ctxKey(acc.ID, peer)]; ok {
			return acc
		}
	}
	if len(c.accs) > 0 {
		return c.accs[0]
	}
	return nil
}

func ctxKey(accountID, peer string) string { return accountID + "\x00" + peer }

func (c *Channel) getCtxTok(accountID, peer string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ctxTok[ctxKey(accountID, peer)]
}

func (c *Channel) setCtxTok(accountID, peer, tok string) {
	if peer == "" || tok == "" {
		return
	}
	c.mu.Lock()
	c.ctxTok[ctxKey(accountID, peer)] = tok
	c.mu.Unlock()
	c.saveContextTokensForAccount(accountID)
}

// Same on-disk layout as @tencent-weixin/openclaw-weixin: accounts/{id}.context-tokens.json
func (c *Channel) loadContextTokensForAccount(accountID string) {
	p := filepath.Join(c.stateRoot, "openclaw-weixin", "accounts", accountID+".context-tokens.json")
	raw, err := os.ReadFile(p)
	if err != nil && deriveRawAccountID(accountID) != "" {
		p2 := filepath.Join(c.stateRoot, "openclaw-weixin", "accounts", deriveRawAccountID(accountID)+".context-tokens.json")
		raw, err = os.ReadFile(p2)
	}
	if err != nil {
		return
	}
	var m map[string]string
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for uid, tok := range m {
		uid, tok = strings.TrimSpace(uid), strings.TrimSpace(tok)
		if uid == "" || tok == "" {
			continue
		}
		c.ctxTok[ctxKey(accountID, uid)] = tok
	}
}

func (c *Channel) saveContextTokensForAccount(accountID string) {
	c.mu.RLock()
	out := make(map[string]string)
	for k, v := range c.ctxTok {
		parts := strings.SplitN(k, "\x00", 2)
		if len(parts) == 2 && parts[0] == accountID && parts[1] != "" && v != "" {
			out[parts[1]] = v
		}
	}
	c.mu.RUnlock()
	p := filepath.Join(c.stateRoot, "openclaw-weixin", "accounts", accountID+".context-tokens.json")
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		logger.WarnCF("weixinclaw", "context-tokens mkdir", map[string]any{"err": err.Error()})
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return
	}
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		logger.WarnCF("weixinclaw", "context-tokens write", map[string]any{"err": err.Error()})
	}
}

func (c *Channel) markSeen(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seen[key]; ok {
		return false
	}
	c.seen[key] = struct{}{}
	return true
}

func (c *Channel) pollLoop(ctx context.Context, acc *weixinAccount) {
	syncPath := filepath.Join(c.stateRoot, "openclaw-weixin", "accounts", acc.ID+".sync.json")
	buf := loadSyncBuf(syncPath)
	if buf == "" && deriveRawAccountID(acc.ID) != "" {
		buf = loadSyncBuf(filepath.Join(c.stateRoot, "openclaw-weixin", "accounts", deriveRawAccountID(acc.ID)+".sync.json"))
	}
	nextTimeout := longPollMS
	failures := 0

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		resp, err := c.doGetUpdates(ctx, acc, buf, nextTimeout)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures++
			logger.WarnCF("weixinclaw", "getUpdates error", map[string]any{"account": acc.ID, "err": err.Error(), "n": failures})
			if failures >= maxFailures {
				time.Sleep(backoffOnFail)
				failures = 0
			} else {
				time.Sleep(retryDelay)
			}
			continue
		}
		failures = 0

		if resp.LongPollingTimeoutMS > 0 {
			nextTimeout = resp.LongPollingTimeoutMS
		}

		if isAPIError(resp) {
			if isSessionExpired(resp) {
				logger.WarnCF("weixinclaw", "session expired, pausing", map[string]any{"account": acc.ID})
				time.Sleep(5 * time.Minute)
				continue
			}
			failures++
			logger.WarnCF("weixinclaw", "getUpdates API error", map[string]any{
				"account": acc.ID, "ret": resp.Ret, "errcode": resp.ErrCode, "msg": resp.ErrMsg,
			})
			if failures >= maxFailures {
				time.Sleep(backoffOnFail)
				failures = 0
			} else {
				time.Sleep(retryDelay)
			}
			continue
		}
		failures = 0

		if resp.GetUpdatesBuf != "" {
			buf = resp.GetUpdatesBuf
			_ = saveSyncBuf(syncPath, buf)
		}

		for _, m := range resp.Msgs {
			c.dispatchInbound(ctx, acc, m)
		}
	}
}

type getUpdatesResp struct {
	Ret                    int             `json:"ret"`
	ErrCode                int             `json:"errcode"`
	ErrMsg                 string          `json:"errmsg"`
	Msgs                   []weixinMessage `json:"msgs"`
	GetUpdatesBuf          string          `json:"get_updates_buf"`
	LongPollingTimeoutMS   int             `json:"longpolling_timeout_ms"`
}

type weixinMessage struct {
	MessageID     int64         `json:"message_id"`
	FromUserID    string        `json:"from_user_id"`
	MessageType   int           `json:"message_type"`
	MessageState  int           `json:"message_state"`
	ContextToken  string        `json:"context_token"`
	ItemList      []messageItem `json:"item_list"`
}

type cdnMedia struct {
	EncryptQuery string `json:"encrypt_query_param"`
	AesKey       string `json:"aes_key"`
	EncryptType  int    `json:"encrypt_type"`
}

type imageItem struct {
	Media      *cdnMedia `json:"media"`
	ThumbMedia *cdnMedia `json:"thumb_media"`
	AesKeyHex  string    `json:"aeskey"`
}

type voiceItem struct {
	Media *cdnMedia `json:"media"`
	Text  string    `json:"text"`
}

type fileItem struct {
	Media    *cdnMedia `json:"media"`
	FileName string    `json:"file_name"`
}

type videoItem struct {
	Media *cdnMedia `json:"media"`
}

type refMessage struct {
	MessageItem *messageItem `json:"message_item"`
	Title       string       `json:"title"`
}

type messageItem struct {
	Type      int         `json:"type"`
	TextItem  *textItem   `json:"text_item"`
	ImageItem *imageItem  `json:"image_item"`
	VoiceItem *voiceItem  `json:"voice_item"`
	FileItem  *fileItem   `json:"file_item"`
	VideoItem *videoItem  `json:"video_item"`
	RefMsg    *refMessage `json:"ref_msg"`
}

type textItem struct {
	Text string `json:"text"`
}

func isAPIError(r getUpdatesResp) bool {
	if r.Ret != 0 {
		return true
	}
	if r.ErrCode != 0 {
		return true
	}
	return false
}

func isSessionExpired(r getUpdatesResp) bool {
	return r.ErrCode == sessionExpiredCode || r.Ret == sessionExpiredCode
}

func (c *Channel) dispatchInbound(ctx context.Context, acc *weixinAccount, m weixinMessage) {
	if m.MessageType != 0 && m.MessageType != 1 {
		return
	}
	if m.MessageState != 2 {
		return
	}
	from := strings.TrimSpace(m.FromUserID)
	if from == "" {
		return
	}
	dedupKey := fmt.Sprintf("%s:%d:%d", acc.ID, m.MessageID, m.MessageType)
	if m.MessageID != 0 {
		if !c.markSeen(dedupKey) {
			return
		}
	}
	tok := strings.TrimSpace(m.ContextToken)
	if tok != "" {
		c.setCtxTok(acc.ID, from, tok)
	}
	text := bodyFromWeixinItems(m.ItemList)
	mi := pickInboundMediaItem(m.ItemList)
	var mediaRefs []string
	if mi != nil {
		data, nameHint, err := fetchDecryptedItemBytes(ctx, mi, c.effectiveCdnBase(), acc.ID)
		if err != nil {
			logger.WarnCF("weixinclaw", "inbound media decrypt/download failed", map[string]any{
				"account": acc.ID, "err": err.Error(),
			})
		} else {
			msgIDStr := ""
			if m.MessageID != 0 {
				msgIDStr = fmt.Sprintf("%d", m.MessageID)
			}
			scope := channels.BuildMediaScope("weixinclaw", from, msgIDStr)
			ref, err := c.storeInboundMedia(scope, mi, data, nameHint)
			if err != nil {
				logger.WarnCF("weixinclaw", "inbound media store failed", map[string]any{"err": err.Error()})
			} else {
				mediaRefs = append(mediaRefs, ref)
			}
		}
	}
	if text == "" && len(mediaRefs) > 0 {
		text = weixinMediaTagForItem(mi)
	}
	if text == "" && len(mediaRefs) == 0 {
		return
	}
	sender := bus.SenderInfo{
		Platform:    "weixinclaw",
		PlatformID:  from,
		CanonicalID: identity.BuildCanonicalID("weixinclaw", from),
	}
	meta := map[string]string{"account_id": acc.ID}
	msgID := ""
	if m.MessageID != 0 {
		msgID = fmt.Sprintf("%d", m.MessageID)
	}
	c.HandleMessage(ctx,
		bus.Peer{Kind: "direct", ID: from},
		msgID, from, from, text, mediaRefs, meta, sender,
	)
}

func loadSyncBuf(p string) string {
	raw, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var w struct {
		Buf string `json:"get_updates_buf"`
	}
	_ = json.Unmarshal(raw, &w)
	return w.Buf
}

func saveSyncBuf(p, buf string) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.Marshal(map[string]string{"get_updates_buf": buf})
	return os.WriteFile(p, b, 0o644)
}

func (c *Channel) doGetUpdates(ctx context.Context, acc *weixinAccount, buf string, timeoutMs int) (getUpdatesResp, error) {
	body := map[string]any{
		"get_updates_buf": buf,
		"base_info":       weixinBaseInfo(),
	}
	// Match api.ts getUpdates: client timeout equals longPollTimeoutMs (no extra slack).
	raw, err := c.postJSON(ctx, acc, "ilink/bot/getupdates", body, time.Duration(timeoutMs)*time.Millisecond)
	if err != nil {
		if isWeixinLongPollTimeout(err) {
			return getUpdatesResp{Ret: 0, GetUpdatesBuf: buf, Msgs: nil}, nil
		}
		return getUpdatesResp{}, err
	}
	var r getUpdatesResp
	if err := json.Unmarshal(raw, &r); err != nil {
		return getUpdatesResp{}, err
	}
	return r, nil
}

func (c *Channel) postJSON(ctx context.Context, acc *weixinAccount, endpoint string, body any, timeout time.Duration) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(strings.TrimSpace(acc.BaseURL), "/")
	u := base + "/" + strings.TrimPrefix(endpoint, "/")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("Authorization", "Bearer "+acc.Token)
	req.Header.Set("X-WECHAT-UIN", randomWechatUIN())
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(b)))
	if c.routeTag != "" {
		req.Header.Set("SKRouteTag", c.routeTag)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return raw, nil
}

func randomWechatUIN() string {
	var n uint32
	_ = binary.Read(rand.Reader, binary.BigEndian, &n)
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

func buildSendBody(to, text, ctxTok string) map[string]any {
	clientID := randomClientID()
	msg := map[string]any{
		"from_user_id":  "",
		"to_user_id":    to,
		"client_id":     clientID,
		"message_type":  2,
		"message_state": 2,
	}
	if strings.TrimSpace(ctxTok) != "" {
		msg["context_token"] = ctxTok
	}
	if text != "" {
		msg["item_list"] = []map[string]any{{
			"type":       1,
			"text_item": map[string]string{"text": text},
		}}
	}
	return map[string]any{
		"msg":       msg,
		"base_info": weixinBaseInfo(),
	}
}

func randomClientID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("anyclaw-%x", b)
}

var (
	reMDCode       = regexp.MustCompile("(?s)```[^\\n]*\\n?([\\s\\S]*?)```")
	reMDImg        = regexp.MustCompile("!\\[[^\\]]*\\]\\([^)]*\\)")
	reMDLink       = regexp.MustCompile("\\[([^\\]]+)\\]\\([^)]*\\)")
	reMDTableSep   = regexp.MustCompile(`(?m)^\|[\s:|-]+\|$`)
	reMDTableRow   = regexp.MustCompile(`(?m)^\|(.+)\|$`)
	reMDInlineCode = regexp.MustCompile("`([^`]+)`")
	reMDBoldStar   = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	reMDBoldUnder  = regexp.MustCompile(`__([^_]+)__`)
)

// stripMarkdownLite follows openclaw-weixin send.ts markdownToPlainText, plus a light pass for **/__/`code` (plugin defers to stripMarkdown).
func stripMarkdownLite(s string) string {
	s = reMDCode.ReplaceAllString(s, "$1")
	s = reMDImg.ReplaceAllString(s, "")
	s = reMDLink.ReplaceAllString(s, "$1")
	s = reMDTableSep.ReplaceAllString(s, "")
	s = reMDTableRow.ReplaceAllStringFunc(s, func(line string) string {
		m := reMDTableRow.FindStringSubmatch(line)
		if len(m) < 2 {
			return line
		}
		cells := strings.Split(m[1], "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		return strings.Join(cells, "  ")
	})
	for i := 0; i < 8; i++ {
		next := reMDBoldStar.ReplaceAllString(s, "$1")
		next = reMDBoldUnder.ReplaceAllString(next, "$1")
		next = reMDInlineCode.ReplaceAllString(next, "$1")
		if next == s {
			break
		}
		s = next
	}
	return strings.TrimSpace(s)
}

// weixinJSONBizError reports ilink logical errors when HTTP status is 200.
func weixinJSONBizError(raw []byte) error {
	var w struct {
		Ret     int    `json:"ret"`
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return nil
	}
	if w.Ret != 0 {
		if msg := strings.TrimSpace(w.ErrMsg); msg != "" {
			return fmt.Errorf("ilink ret=%d: %s", w.Ret, msg)
		}
		return fmt.Errorf("ilink ret=%d", w.Ret)
	}
	if w.ErrCode != 0 {
		if msg := strings.TrimSpace(w.ErrMsg); msg != "" {
			return fmt.Errorf("ilink errcode=%d: %s", w.ErrCode, msg)
		}
		return fmt.Errorf("ilink errcode=%d", w.ErrCode)
	}
	return nil
}

func isWeixinLongPollTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// StartTyping implements channels.TypingCapable (getconfig + sendtyping).
func (c *Channel) StartTyping(ctx context.Context, chatID string) (func(), error) {
	acc := c.pickAccountForPeer(chatID)
	if acc == nil {
		return func() {}, nil
	}
	ctxTok := c.getCtxTok(acc.ID, chatID)
	cfgBody := map[string]any{
		"ilink_user_id": chatID,
		"base_info":     weixinBaseInfo(),
	}
	if strings.TrimSpace(ctxTok) != "" {
		cfgBody["context_token"] = ctxTok
	}
	raw, err := c.postJSON(ctx, acc, "ilink/bot/getconfig", cfgBody, 10*time.Second)
	if err != nil {
		return func() {}, nil
	}
	var cfg struct {
		Ret          int    `json:"ret"`
		ErrCode      int    `json:"errcode"`
		TypingTicket string `json:"typing_ticket"`
	}
	if json.Unmarshal(raw, &cfg) != nil || cfg.Ret != 0 || cfg.ErrCode != 0 || strings.TrimSpace(cfg.TypingTicket) == "" {
		return func() {}, nil
	}
	ticket := strings.TrimSpace(cfg.TypingTicket)
	send := func(status int) {
		ctx2, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		b := map[string]any{
			"ilink_user_id": chatID,
			"typing_ticket": ticket,
			"status":        status,
			"base_info":     weixinBaseInfo(),
		}
		_, _ = c.postJSON(ctx2, acc, "ilink/bot/sendtyping", b, 10*time.Second)
	}
	send(1)
	return func() { send(2) }, nil
}
