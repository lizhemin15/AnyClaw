package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

// KeepAlive 保活服务：定期探测各渠道连通性，成功则清除冷却、失败则标记冷却，
// 供调度器据此切换渠道。
type KeepAlive struct {
	configPath string
	scheduler  *ModelScheduler
	client     *http.Client
	interval   time.Duration
	stopCh     chan struct{}
	mu         sync.Mutex
}

// NewKeepAlive 创建保活服务，interval 为探测间隔（如 5*time.Minute）
func NewKeepAlive(configPath string, scheduler *ModelScheduler, interval time.Duration) *KeepAlive {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &KeepAlive{
		configPath: configPath,
		scheduler:  scheduler,
		client:     &http.Client{Timeout: 15 * time.Second},
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动保活协程
func (k *KeepAlive) Start() {
	k.probeAll() // 启动时立即探测一次，避免首次请求打到已挂渠道
	go k.loop()
	log.Printf("[llm] keepalive started, interval=%v", k.interval)
}

// Stop 停止保活
func (k *KeepAlive) Stop() {
	k.mu.Lock()
	defer k.mu.Unlock()
	select {
	case <-k.stopCh:
		return
	default:
		close(k.stopCh)
	}
}

func (k *KeepAlive) loop() {
	ticker := time.NewTicker(k.interval)
	defer ticker.Stop()
	for {
		select {
		case <-k.stopCh:
			return
		case <-ticker.C:
			k.probeAll()
		}
	}
}

func (k *KeepAlive) probeAll() {
	cfg, err := config.Load(k.configPath)
	if err != nil || cfg == nil {
		return
	}
	type target struct {
		channelID, apiBase, apiKey, model string
	}
	var targets []target
	seen := make(map[string]bool)
	// 仅对用户启用的渠道做保活检测；用户手动关闭的不参与检测、不自动禁用
	for _, ch := range cfg.Channels {
		if !ch.Enabled || ch.APIKey == "" {
			continue
		}
		base := ch.APIBase
		if base == "" {
			base = "https://api.openai.com/v1"
		}
		base = strings.TrimSuffix(base, "/")
		model := "gpt-4o"
		if len(ch.Models) > 0 {
			for _, m := range ch.Models {
				if m.Name != "" {
					model = m.Name
					break
				}
			}
		}
		key := ch.ID + "|" + base
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, target{channelID: ch.ID, apiBase: base, apiKey: ch.APIKey, model: model})
	}
	for _, t := range targets {
		go k.probeOne(t.channelID, t.apiBase, t.apiKey, t.model)
	}
}

func (k *KeepAlive) probeOne(channelID, apiBase, apiKey, model string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "."},
		},
		"max_tokens": 1,
	}
	bodyBytes, _ := json.Marshal(body)
	reqURL := apiBase + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	if u, err := url.Parse(reqURL); err == nil && u.Host != "" {
		host := u.Hostname()
		if p := u.Port(); p != "" && p != "443" && p != "80" {
			host = u.Host
		}
		req.Host = host
	}
	resp, err := k.client.Do(req)
	ep := config.ChannelEndpoint{ChannelID: channelID, APIBase: apiBase}
	if err != nil {
		log.Printf("[llm] keepalive probe %s failed: %v", apiBase, err)
		if k.scheduler != nil {
			k.scheduler.RecordFailureUntil(ep, time.Now().Add(CooldownTransient))
		}
		return
	}
	rb, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		if k.scheduler != nil {
			k.scheduler.ClearFailure(ep)
		}
		return
	}
	// 5xx / 429：记录冷却，供调度器切换
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		until := cooldownUntil(resp.StatusCode, rb)
		log.Printf("[llm] keepalive probe %s status=%d cooldown_until=%v", apiBase, resp.StatusCode, until.Format("2006-01-02 15:04"))
		if k.scheduler != nil {
			k.scheduler.RecordFailureUntil(ep, until)
		}
	}
}
