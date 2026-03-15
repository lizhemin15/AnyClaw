package proxy

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

// Handler 代理处理器，用于绕过 CORS 获取 COS 等外部资源
type Handler struct {
	configPath string
}

// New 创建 Handler
func New(configPath string) *Handler {
	return &Handler{configPath: configPath}
}

// isAllowedHost 校验 URL 是否在白名单内
func isAllowedHost(targetURL *url.URL, cfg *config.Config) bool {
	if targetURL.Scheme != "https" {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(targetURL.Hostname()))
	// 腾讯云 COS 默认域名
	if strings.HasSuffix(host, ".myqcloud.com") && strings.Contains(host, ".cos.") {
		return true
	}
	// 配置的自定义域名
	if cfg != nil && cfg.COS != nil && cfg.COS.Domain != "" {
		domain := strings.ToLower(strings.TrimSpace(cfg.COS.Domain))
		domain = strings.TrimPrefix(domain, "https://")
		domain = strings.TrimPrefix(domain, "http://")
		domain = strings.TrimSuffix(domain, "/")
		if idx := strings.Index(domain, "/"); idx >= 0 {
			domain = domain[:idx]
		}
		if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
			return true
		}
	}
	return false
}

// HandleProxy GET /api/proxy?url=xxx 代理请求，需 JWT 鉴权
func (h *Handler) HandleProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	rawURL := r.URL.Query().Get("url")
	if rawURL == "" {
		http.Error(w, `{"error":"url required"}`, http.StatusBadRequest)
		return
	}
	targetURL, err := url.Parse(rawURL)
	if err != nil {
		http.Error(w, `{"error":"invalid url"}`, http.StatusBadRequest)
		return
	}
	cfg, _ := config.Load(h.configPath)
	if !isAllowedHost(targetURL, cfg) {
		http.Error(w, `{"error":"url not allowed"}`, http.StatusForbidden)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, rawURL, nil)
	if err != nil {
		http.Error(w, `{"error":"failed to create request"}`, http.StatusInternalServerError)
		return
	}
	req.Header.Set("User-Agent", "AnyClaw-Proxy/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, `{"error":"fetch failed"}`, http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, `{"error":"upstream error"}`, resp.StatusCode)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	io.Copy(w, resp.Body)
}
