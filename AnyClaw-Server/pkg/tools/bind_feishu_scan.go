package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	qrterminal "github.com/mdp/qrterminal/v3"

	feishuchan "github.com/anyclaw/anyclaw-server/pkg/channels/feishu"
	"github.com/anyclaw/anyclaw-server/pkg/feishu/onboard"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
)

// BindFeishuScanTool starts the same device-registration flow as the official
// `npx @larksuite/openclaw-lark install` "new bot" path: user scans a QR (URL)
// in Feishu, then AnyClaw polls for app credentials and writes config.
type BindFeishuScanTool struct {
	mu      sync.Mutex
	running bool
}

var _ AsyncExecutor = (*BindFeishuScanTool)(nil)

func NewBindFeishuScanTool() *BindFeishuScanTool {
	return &BindFeishuScanTool{}
}

func (t *BindFeishuScanTool) Name() string {
	return "bind_feishu_scan"
}

func (t *BindFeishuScanTool) Description() string {
	return `Start Feishu binding without manually typing app_id/app_secret: opens the official "create bot via scan" device flow (same API as @larksuite/openclaw-lark install / 新建机器人). Sends the scan URL and a terminal-style QR to the user, waits in the background until the user finishes in Feishu, then writes channels.feishu in config and restarts the gateway (Unix). Prefer this when the user says they want to bind Feishu / 绑定飞书 / 扫码绑定.`
}

func (t *BindFeishuScanTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"env": map[string]any{
				"type":        "string",
				"description": "Accounts environment: prod (default), boe, or pre",
				"enum":        []string{"prod", "boe", "pre"},
			},
			"lane": map[string]any{
				"type":        "string",
				"description": "Optional traffic lane header (x-tt-env), same as official CLI --lane",
			},
		},
	}
}

func (t *BindFeishuScanTool) Execute(ctx context.Context, _ map[string]any) *ToolResult {
	_ = ctx
	return ErrorResult("bind_feishu_scan must run from the agent with async support (conversation only)")
}

func (t *BindFeishuScanTool) ExecuteAsync(ctx context.Context, args map[string]any, cb AsyncCallback) *ToolResult {
	if cb == nil {
		return ErrorResult("internal error: missing async callback")
	}

	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return ErrorResult("已有飞书扫码绑定正在进行，请等待完成后再试")
	}
	t.running = true
	t.mu.Unlock()

	env, _ := args["env"].(string)
	lane, _ := args["lane"].(string)
	if strings.TrimSpace(env) == "" {
		env = "prod"
	}

	client := onboard.NewClient(env)
	client.Lane = strings.TrimSpace(lane)

	initRes, err := client.Init(ctx)
	if err != nil {
		t.clearRunning()
		return ErrorResult("飞书注册初始化失败: " + err.Error())
	}
	if len(initRes.SupportedAuthMethods) > 0 {
		okMethod := false
		for _, m := range initRes.SupportedAuthMethods {
			if m == "client_secret" {
				okMethod = true
				break
			}
		}
		if !okMethod {
			t.clearRunning()
			return ErrorResult("当前环境不支持 client_secret 注册，请升级飞书 onboard 工具或改用开放平台手动创建应用")
		}
	}

	beginRes, err := client.Begin(ctx)
	if err != nil {
		t.clearRunning()
		return ErrorResult("无法开始扫码绑定: " + err.Error())
	}

	qrText := renderFeishuQR(beginRes.VerificationURIComplete)
	forUser := strings.TrimSpace(fmt.Sprintf(`已按「新建机器人」流程发起飞书官方扫码绑定（与 npx @larksuite/openclaw-lark install 使用同一注册接口）。

请用飞书客户端扫描下方二维码，或打开链接完成创建机器人：
%s

完成后 AnyClaw 会在后台自动写入 app_id / app_secret 并重启网关（Linux/macOS）。Windows 上请在本机手动重启 openclaw gateway。

验证：在飞书对话中发送 /feishu start 查看版本；需要更多能力可发 /feishu auth 做批量授权。`,
		qrText+"\n\n"+beginRes.VerificationURIComplete))

	go t.pollUntilDone(ctx, client, env, beginRes, cb)

	return &ToolResult{
		ForLLM:  "已向用户发送飞书扫码绑定说明与二维码（新建机器人流程）。正在后台轮询注册结果；成功后会写入 config 并触发重启，你会收到系统消息。若超时或失败，可建议用户重试或使用 update_feishu_config 手动填凭证。",
		ForUser: forUser,
		Silent:  false,
		Async:   true,
	}
}

func (t *BindFeishuScanTool) clearRunning() {
	t.mu.Lock()
	t.running = false
	t.mu.Unlock()
}

func renderFeishuQR(uri string) string {
	var buf bytes.Buffer
	qrterminal.GenerateWithConfig(uri, qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     &buf,
		HalfBlocks: true,
	})
	return buf.String()
}

func (t *BindFeishuScanTool) pollUntilDone(ctx context.Context, client *onboard.Client, env string, begin *onboard.BeginResponse, cb AsyncCallback) {
	defer t.clearRunning()

	deadline := time.Now().Add(time.Duration(begin.ExpireIn) * time.Second)
	interval := time.Duration(begin.Interval) * time.Second
	if interval < time.Second {
		interval = time.Second
	}
	domainSwitched := false
	deviceCode := begin.DeviceCode

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			cb(context.Background(), ErrorResult("飞书绑定已取消"))
			return
		}

		pr, err := client.Poll(ctx, deviceCode)
		if err != nil {
			logger.ErrorCF("feishu_scan", "poll failed", map[string]any{"error": err.Error()})
			cb(context.Background(), ErrorResult("轮询注册状态失败: "+err.Error()))
			return
		}

		if pr.ClientID != "" && pr.ClientSecret != "" {
			var extra []string
			if id := strings.TrimSpace(pr.UserInfo.OpenID); id != "" {
				extra = append(extra, id)
			}
			if err := persistFeishuCredentials(ctx, pr.ClientID, pr.ClientSecret, extra); err != nil {
				cb(context.Background(), ErrorResult("已获取机器人凭证但写入配置失败: "+err.Error()))
				return
			}
			msg := feishuchan.BindingSuccessMessage() + " 若需飞书插件中的文档/日历等能力，可在飞书中发送 /feishu auth 完成授权。"
			cb(context.Background(), UserResult(msg))
			return
		}

		if pr.UserInfo.TenantBrand == "lark" && !domainSwitched {
			client.UseLarkInternational(env)
			domainSwitched = true
			continue
		}

		switch pr.Error {
		case "", "authorization_pending":
			// continue
		case "slow_down":
			interval += 5 * time.Second
		case "access_denied":
			cb(context.Background(), ErrorResult("用户拒绝授权，绑定已中止"))
			return
		case "expired_token":
			cb(context.Background(), ErrorResult("会话已过期，请让用户重新说「绑定飞书」再试一次"))
			return
		default:
			desc := strings.TrimSpace(pr.ErrorDescription)
			if desc != "" {
				cb(context.Background(), ErrorResult("注册失败: "+pr.Error+" — "+desc))
			} else {
				cb(context.Background(), ErrorResult("注册失败: "+pr.Error))
			}
			return
		}

		select {
		case <-ctx.Done():
			cb(context.Background(), ErrorResult("飞书绑定已取消"))
			return
		case <-time.After(interval):
		}
	}

	cb(context.Background(), ErrorResult("等待扫码超时。请重试绑定，或在飞书开放平台手动创建应用后使用 update_feishu_config"))
}
