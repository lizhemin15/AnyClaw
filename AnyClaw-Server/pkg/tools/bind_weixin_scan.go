package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"
	"rsc.io/qr"

	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/media"
)

// BindWeixinScanTool runs the same WeChat QR login flow as @tencent-weixin/openclaw-weixin
// (ilink get_bot_qrcode + get_qrcode_status) and writes credentials under the OpenClaw-style
// state directory for the official Weixin channel plugin.
type BindWeixinScanTool struct {
	mu         sync.Mutex
	running    bool
	mediaStore media.MediaStore
}

func (t *BindWeixinScanTool) SetMediaStore(store media.MediaStore) {
	t.mu.Lock()
	t.mediaStore = store
	t.mu.Unlock()
}

var _ AsyncExecutor = (*BindWeixinScanTool)(nil)

func NewBindWeixinScanTool() *BindWeixinScanTool {
	return &BindWeixinScanTool{}
}

func (t *BindWeixinScanTool) Name() string {
	return "bind_weixin_scan"
}

func (t *BindWeixinScanTool) Description() string {
	return `Start WeChat (微信 ClawBot) binding via official ilink QR flow (same APIs as @tencent-weixin/openclaw-weixin). Sends QR + link, saves token into config.json (channels.weixin_claw.accounts, same persistence as Feishu) and mirrors openclaw-weixin/ next to config, enables weixin_claw, triggers gateway restart on Unix. Use for 绑定微信 / 微信扫码.`
}

func (t *BindWeixinScanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"base_url": map[string]any{
				"type":        "string",
				"description": "ilink API base URL (default https://ilinkai.weixin.qq.com)",
			},
			"route_tag": map[string]any{
				"type":        "string",
				"description": "Optional SKRouteTag header (same as plugin channels.openclaw-weixin.routeTag)",
			},
			"bot_type": map[string]any{
				"type":        "string",
				"description": "bot_type query param for get_bot_qrcode (default 3)",
			},
			"timeout_ms": map[string]any{
				"type":        "number",
				"description": "Max wait for scan+confirm in milliseconds (default 480000)",
			},
		},
	}
}

func (t *BindWeixinScanTool) Execute(ctx context.Context, _ map[string]any) *ToolResult {
	_ = ctx
	return ErrorResult("bind_weixin_scan must run from the agent with async support (conversation only)")
}

func (t *BindWeixinScanTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	if cb == nil {
		return ErrorResult("internal error: missing async callback")
	}

	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return ErrorResult("已有微信扫码绑定正在进行，请等待完成后再试")
	}
	t.running = true
	t.mu.Unlock()

	baseURL, _ := args["base_url"].(string)
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		baseURL = weixinDefaultBaseURL
	}
	routeTag, _ := args["route_tag"].(string)
	routeTag = strings.TrimSpace(routeTag)
	botType, _ := args["bot_type"].(string)
	botType = strings.TrimSpace(botType)
	if botType == "" {
		botType = weixinDefaultBotType
	}
	timeoutMs := 480_000.0
	if v, ok := args["timeout_ms"].(float64); ok && v >= 10_000 {
		timeoutMs = v
	}

	qrData, err := weixinFetchBotQR(ctx, baseURL, botType, routeTag)
	if err != nil {
		t.clearRunning()
		return ErrorResult("获取微信绑定二维码失败: " + err.Error())
	}
	qrcode := strings.TrimSpace(qrData.Qrcode)
	qrURL := strings.TrimSpace(qrData.QrcodeImgContent)
	if qrcode == "" || qrURL == "" {
		t.clearRunning()
		return ErrorResult("微信接口未返回有效二维码，请稍后重试")
	}

	ch, cid := ToolChannel(ctx), ToolChatID(ctx)
	scope := fmt.Sprintf("tool:bind_weixin_scan:%s:%s", ch, cid)

	var mediaRefs []string
	if t.mediaStore != nil && ch != "" && cid != "" && qrURL != "" {
		if ref, perr := weixinStoreQRPNG(t.mediaStore, scope, qrURL); perr != nil {
			logger.WarnCF("weixin_scan", "store PNG QR failed", map[string]any{"error": perr.Error()})
		} else if ref != "" {
			mediaRefs = append(mediaRefs, ref)
		}
	}

	scanHint := "请用微信扫描**同时发送的二维码图片**，或在浏览器打开下方链接扫码："
	if len(mediaRefs) == 0 {
		scanHint = "请用微信扫描下方终端风格二维码（网页端若排版错乱请改用链接），或复制链接到浏览器打开："
	}
	qrBlock := ""
	if len(mediaRefs) == 0 {
		qrBlock = weixinRenderTerminalQR(qrURL) + "\n\n"
	}

	forUser := strings.TrimSpace(fmt.Sprintf(`已发起微信 ClawBot 扫码绑定（与官方插件 @tencent-weixin/openclaw-weixin 使用相同 ilink 接口）。

%s
%s
%s

凭证将写入 **config.json**（与飞书相同，容器挂载该文件即可持久化），并同步到 %s/openclaw-weixin/。
绑定成功后会自动打开 **channels.weixin_claw** 并重启网关（Linux/macOS）；Windows 请手动重启 anyclaw gateway。
若提示微信版本过低，请按微信引导更新后再扫。`,
		scanHint,
		qrBlock,
		qrURL,
		filepath.Clean(config.ConfigPersistenceDir())))

	deadline := time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)

	go t.waitWeixinLogin(ctx, cb, weixinWaitParams{
		BaseURL:   baseURL,
		RouteTag:  routeTag,
		BotType:   botType,
		Qrcode:    qrcode,
		QRURL:     qrURL,
		Deadline:  deadline,
		MediaRefs: mediaRefs,
		Scope:     scope,
		Channel:   ch,
		ChatID:    cid,
	})

	return &ToolResult{
		ForLLM:  "已向用户发送微信扫码说明与二维码。后台长轮询等待确认；成功后会写入凭证、启用 weixin_claw 通道并触发重启。",
		ForUser: forUser,
		Media:   mediaRefs,
		Silent:  false,
		Async:   true,
	}
}

func (t *BindWeixinScanTool) clearRunning() {
	t.mu.Lock()
	t.running = false
	t.mu.Unlock()
}

type weixinWaitParams struct {
	BaseURL   string
	RouteTag  string
	BotType   string
	Qrcode    string
	QRURL     string
	Deadline  time.Time
	MediaRefs []string
	Scope     string
	Channel   string
	ChatID    string
}

func (t *BindWeixinScanTool) waitWeixinLogin(ctx context.Context, cb AsyncCallback, p weixinWaitParams) {
	defer t.clearRunning()

	qrRefreshCount := 1
	scannedPrinted := false
	qrcode := p.Qrcode
	qrURL := p.QRURL

	for time.Now().Before(p.Deadline) {
		if ctx.Err() != nil {
			cb(context.Background(), ErrorResult("微信绑定已取消"))
			return
		}

		st, err := weixinPollQRStatus(ctx, p.BaseURL, qrcode, p.RouteTag)
		if err != nil {
			logger.ErrorCF("weixin_scan", "poll failed", map[string]any{"error": err.Error()})
			cb(context.Background(), ErrorResult("轮询扫码状态失败: "+err.Error()))
			return
		}

		switch st.Status {
		case "wait":
			// continue
		case "scaned":
			if !scannedPrinted {
				scannedPrinted = true
				cb(context.Background(), &ToolResult{
					ForLLM:  "用户已在微信中扫码，等待用户在手机端确认授权。",
					ForUser: "已检测到扫码，请在微信上继续确认授权。",
					Silent:  false,
					Async:   false,
				})
			}
		case "expired":
			qrRefreshCount++
			if qrRefreshCount > weixinMaxQRRefresh {
				cb(context.Background(), ErrorResult("二维码多次过期，请重新说「绑定微信」再试"))
				return
			}
			qrData, err := weixinFetchBotQR(ctx, p.BaseURL, p.BotType, p.RouteTag)
			if err != nil {
				cb(context.Background(), ErrorResult("刷新二维码失败: "+err.Error()))
				return
			}
			qrcode = strings.TrimSpace(qrData.Qrcode)
			qrURL = strings.TrimSpace(qrData.QrcodeImgContent)
			if qrcode == "" || qrURL == "" {
				cb(context.Background(), ErrorResult("刷新二维码时接口返回异常"))
				return
			}
			scannedPrinted = false
			var newRefs []string
			if t.mediaStore != nil && p.Channel != "" && p.ChatID != "" {
				if ref, perr := weixinStoreQRPNG(t.mediaStore, p.Scope, qrURL); perr == nil && ref != "" {
					newRefs = append(newRefs, ref)
				}
			}
			scanPart := "请扫描**本消息附带的二维码图片**，或打开链接："
			if len(newRefs) == 0 {
				scanPart = weixinRenderTerminalQR(qrURL) + "\n\n或打开链接："
			}
			msg := strings.TrimSpace(fmt.Sprintf(
				"上一张二维码已过期，已生成新码（%d/%d）。请用微信重新扫描。\n\n%s\n%s",
				qrRefreshCount, weixinMaxQRRefresh, scanPart, qrURL))
			cb(context.Background(), &ToolResult{
				ForLLM:  "二维码过期已自动刷新，已提示用户重新扫描。",
				ForUser: msg,
				Media:   newRefs,
				Silent:  false,
				Async:   false,
			})
		case "confirmed":
			if strings.TrimSpace(st.IlinkBotID) == "" {
				cb(context.Background(), ErrorResult("登录确认成功但未返回机器人 ID，请重试"))
				return
			}
			if strings.TrimSpace(st.BotToken) == "" {
				cb(context.Background(), ErrorResult("登录确认成功但未返回 token，请重试"))
				return
			}
			normID := normalizeWeixinAccountID(st.IlinkBotID)
			baseSave := strings.TrimSpace(st.BaseURL)
			if baseSave == "" {
				baseSave = p.BaseURL
			}
			userID := strings.TrimSpace(st.IlinkUserID)
			if err := persistWeixinClawBindingToConfig(normID, st.BotToken, baseSave, userID); err != nil {
				cb(context.Background(), ErrorResult("已获取凭证但写入 config.json 失败: "+err.Error()))
				return
			}
			if err := persistOpenClawWeixinAccount(config.ConfigPersistenceDir(), normID, st.BotToken, baseSave, userID); err != nil {
				logger.WarnCF("weixin_scan", "mirror openclaw-weixin dir failed", map[string]any{"error": err.Error()})
			}
			scheduleRestart()
			okMsg := strings.TrimSpace(fmt.Sprintf(`✅ 微信绑定成功，账号已保存（%s），并已启用原生 **weixin_claw** 通道。

网关将在数秒后自动重启（Linux/macOS）以开始接收微信消息；Windows 请手动重启 anyclaw。`,
				normID))
			cb(context.Background(), UserResult(okMsg))
			return
		default:
			cb(context.Background(), ErrorResult("未知扫码状态: "+st.Status))
			return
		}

		select {
		case <-ctx.Done():
			cb(context.Background(), ErrorResult("微信绑定已取消"))
			return
		case <-time.After(time.Second):
		}
	}

	cb(context.Background(), ErrorResult("等待扫码或确认超时，请重试绑定微信"))
}

const (
	weixinDefaultBaseURL = "https://ilinkai.weixin.qq.com"
	weixinDefaultBotType = "3"
	weixinPollTimeout    = 36 * time.Second
	weixinMaxQRRefresh   = 3
)

type weixinQRPayload struct {
	Qrcode             string `json:"qrcode"`
	QrcodeImgContent   string `json:"qrcode_img_content"`
}

type weixinStatusPayload struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token"`
	IlinkBotID  string `json:"ilink_bot_id"`
	BaseURL     string `json:"baseurl"`
	IlinkUserID string `json:"ilink_user_id"`
}

func weixinTrimBase(base string) string {
	s := strings.TrimRight(strings.TrimSpace(base), "/")
	if s == "" {
		return weixinDefaultBaseURL
	}
	return s
}

func weixinFetchBotQR(ctx context.Context, baseURL, botType, routeTag string) (*weixinQRPayload, error) {
	b := weixinTrimBase(baseURL)
	q := url.Values{}
	q.Set("bot_type", botType)
	full := b + "/ilink/bot/get_bot_qrcode?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	if routeTag != "" {
		req.Header.Set("SKRouteTag", routeTag)
	}

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out weixinQRPayload
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func weixinPollQRStatus(ctx context.Context, baseURL, qrcode, routeTag string) (*weixinStatusPayload, error) {
	b := weixinTrimBase(baseURL)
	q := url.Values{}
	q.Set("qrcode", qrcode)
	full := b + "/ilink/bot/get_qrcode_status?" + q.Encode()

	pollCtx, cancel := context.WithTimeout(ctx, weixinPollTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, full, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("iLink-App-ClientVersion", "1")
	if routeTag != "" {
		req.Header.Set("SKRouteTag", routeTag)
	}

	client := &http.Client{Timeout: weixinPollTimeout + time.Second}
	resp, err := client.Do(req)
	if err != nil {
		if pollCtx.Err() == context.DeadlineExceeded {
			return &weixinStatusPayload{Status: "wait"}, nil
		}
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
	var out weixinStatusPayload
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.Status == "" {
		return &weixinStatusPayload{Status: "wait"}, nil
	}
	return &out, nil
}

func normalizeWeixinAccountID(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.ReplaceAll(s, "@", "-")
	s = strings.ReplaceAll(s, ".", "-")
	return s
}

func persistOpenClawWeixinAccount(stateRoot, accountID, token, baseURL, userID string) error {
	root := filepath.Join(stateRoot, "openclaw-weixin")
	accDir := filepath.Join(root, "accounts")
	if err := os.MkdirAll(accDir, 0o700); err != nil {
		return err
	}

	type accountFile struct {
		Token   string `json:"token,omitempty"`
		SavedAt string `json:"savedAt,omitempty"`
		BaseURL string `json:"baseUrl,omitempty"`
		UserID  string `json:"userId,omitempty"`
	}
	data := accountFile{
		Token:   strings.TrimSpace(token),
		SavedAt: time.Now().UTC().Format(time.RFC3339),
		BaseURL: strings.TrimSpace(baseURL),
	}
	if userID != "" {
		data.UserID = userID
	}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	accPath := filepath.Join(accDir, accountID+".json")
	if err := os.WriteFile(accPath, raw, 0o600); err != nil {
		return err
	}

	indexPath := filepath.Join(root, "accounts.json")
	ids := []string{}
	if prev, err := os.ReadFile(indexPath); err == nil {
		_ = json.Unmarshal(prev, &ids)
	}
	if !stringSliceContains(ids, accountID) {
		ids = append(ids, accountID)
	}
	idxBytes, err := json.MarshalIndent(ids, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, idxBytes, 0o644)
}

func stringSliceContains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func weixinStoreQRPNG(store media.MediaStore, scope, uri string) (string, error) {
	pngPath := filepath.Join(os.TempDir(), fmt.Sprintf("anyclaw-weixin-qr-%d.png", time.Now().UnixNano()))
	if err := weixinWriteQRPNG(pngPath, uri); err != nil {
		return "", err
	}
	ref, err := store.Store(pngPath, media.MediaMeta{
		Filename:    "weixin-bind-qr.png",
		ContentType: "image/png",
		Source:      "tool:bind_weixin_scan",
	}, scope)
	if err != nil {
		_ = os.Remove(pngPath)
		return "", err
	}
	return ref, nil
}

func weixinWriteQRPNG(path, uri string) error {
	code, err := qr.Encode(uri, qr.M)
	if err != nil {
		return err
	}
	code.Scale = 12
	img := weixinQRToRGBA(code)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func weixinQRToRGBA(c *qr.Code) *image.RGBA {
	if c.Scale < 1 {
		c.Scale = 8
	}
	w := c.Size * c.Scale
	h := c.Size * c.Scale
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		my := y / c.Scale
		for x := 0; x < w; x++ {
			mx := x / c.Scale
			if c.Black(mx, my) {
				img.Set(x, y, color.Black)
			} else {
				img.Set(x, y, color.White)
			}
		}
	}
	return img
}

func weixinRenderTerminalQR(uri string) string {
	var buf bytes.Buffer
	qrterminal.GenerateWithConfig(uri, qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     &buf,
		HalfBlocks: true,
	})
	return buf.String()
}
