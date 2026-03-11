package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

// KeepAlive 保活服务：定期向各渠道发送最小请求，保持连接活跃
type KeepAlive struct {
	configPath string
	client     *http.Client
	interval   time.Duration
	stopCh     chan struct{}
	mu         sync.Mutex
}

// NewKeepAlive 创建保活服务，interval 为探测间隔（如 5*time.Minute）
func NewKeepAlive(configPath string, interval time.Duration) *KeepAlive {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &KeepAlive{
		configPath: configPath,
		client:     &http.Client{Timeout: 15 * time.Second},
		interval:   interval,
		stopCh:     make(chan struct{}),
	}
}

// Start 启动保活协程
func (k *KeepAlive) Start() {
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
	// 收集所有已启用渠道的 (apiBase, apiKey, model) 用于保活
	type target struct {
		apiBase, apiKey, model string
	}
	var targets []target
	seen := make(map[string]bool)
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
		targets = append(targets, target{apiBase: base, apiKey: ch.APIKey, model: model})
	}
	for _, t := range targets {
		go k.probeOne(t.apiBase, t.apiKey, t.model)
	}
}

func (k *KeepAlive) probeOne(apiBase, apiKey, model string) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
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
	if err != nil {
		log.Printf("[llm] keepalive probe %s failed: %v", apiBase, err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[llm] keepalive probe %s status=%d", apiBase, resp.StatusCode)
	}
}
