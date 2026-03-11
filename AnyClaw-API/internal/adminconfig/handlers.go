package adminconfig

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/mail"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type Handler struct {
	configPath string
}

func New(configPath string) *Handler {
	return &Handler{configPath: configPath}
}

func (h *Handler) GetConfig(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}
	channels := cfg.Channels
	if channels == nil {
		channels = []config.Channel{}
	}
	// Mask API keys for response
	out := make([]map[string]any, len(channels))
	for i, ch := range channels {
		out[i] = map[string]any{
			"id":       ch.ID,
			"name":     ch.Name,
			"api_key":  config.MaskAPIKey(ch.APIKey),
			"api_base": ch.APIBase,
			"enabled":  ch.Enabled,
			"models":   ch.Models,
		}
	}
	resp := map[string]any{"channels": out}
	if cfg.SMTP != nil {
		smtp := map[string]any{
			"host": cfg.SMTP.Host,
			"port": cfg.SMTP.Port,
			"user": cfg.SMTP.User,
			"pass": config.MaskAPIKey(cfg.SMTP.Pass),
			"from": cfg.SMTP.From,
		}
		resp["smtp"] = smtp
	}
	if cfg.Payment != nil {
		payment := map[string]any{"plans": cfg.Payment.Plans}
		if cfg.Payment.Alipay != nil {
			payment["alipay"] = map[string]any{
				"enabled":            cfg.Payment.Alipay.Enabled,
				"app_id":             cfg.Payment.Alipay.AppID,
				"private_key":       config.MaskAPIKey(cfg.Payment.Alipay.PrivateKey),
				"alipay_public_key": config.MaskAPIKey(cfg.Payment.Alipay.AlipayPubKey),
				"is_sandbox":        cfg.Payment.Alipay.IsSandbox,
			}
		}
		if cfg.Payment.Wechat != nil {
			payment["wechat"] = map[string]any{
				"enabled":      cfg.Payment.Wechat.Enabled,
				"app_id":      cfg.Payment.Wechat.AppID,
				"mch_id":      cfg.Payment.Wechat.MchID,
				"api_v3_key":  config.MaskAPIKey(cfg.Payment.Wechat.APIv3Key),
				"serial_no":   cfg.Payment.Wechat.SerialNo,
				"private_key": config.MaskAPIKey(cfg.Payment.Wechat.PrivateKey),
			}
		}
		if cfg.Payment.Yungouos != nil {
			yg := map[string]any{}
			if cfg.Payment.Yungouos.Wechat != nil {
				yg["wechat"] = map[string]any{
					"enabled": cfg.Payment.Yungouos.Wechat.Enabled,
					"mch_id":  cfg.Payment.Yungouos.Wechat.MchID,
					"key":     config.MaskAPIKey(cfg.Payment.Yungouos.Wechat.Key),
				}
			}
			if cfg.Payment.Yungouos.Alipay != nil {
				yg["alipay"] = map[string]any{
					"enabled": cfg.Payment.Yungouos.Alipay.Enabled,
					"mch_id":  cfg.Payment.Yungouos.Alipay.MchID,
					"key":     config.MaskAPIKey(cfg.Payment.Yungouos.Alipay.Key),
				}
			}
			payment["yungouos"] = yg
		}
		resp["payment"] = payment
	}
	if cfg.Energy != nil {
		resp["energy"] = cfg.Energy
	} else {
		def := config.GetEnergyDefaults()
		resp["energy"] = def
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) PutConfig(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req struct {
		Channels []config.Channel      `json:"channels"`
		SMTP     *config.SMTPConfig    `json:"smtp"`
		Payment  *config.PaymentConfig `json:"payment"`
		Energy   *config.EnergyConfig  `json:"energy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil {
		http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
		return
	}
	// Merge: preserve existing api_key if client sent masked value
	channels := req.Channels
	if channels == nil {
		channels = []config.Channel{}
	}
	existing := make(map[string]string)
	for _, ch := range cfg.Channels {
		existing[ch.ID] = ch.APIKey
	}
	for i := range channels {
		if k, ok := existing[channels[i].ID]; ok && (channels[i].APIKey == "" || strings.HasPrefix(channels[i].APIKey, "****")) {
			channels[i].APIKey = k
		}
	}
	// Merge SMTP: preserve pass if client sent masked value; clear if host empty
	smtp := req.SMTP
	if smtp != nil && strings.TrimSpace(smtp.Host) == "" {
		smtp = nil
	} else if smtp != nil && cfg.SMTP != nil && (smtp.Pass == "" || strings.HasPrefix(smtp.Pass, "****")) {
		smtp.Pass = cfg.SMTP.Pass
	}
	// Merge Payment: preserve secrets if client sent masked value; keep existing if not sent
	payment := req.Payment
	if payment == nil {
		payment = cfg.Payment
	} else if cfg.Payment != nil {
		if payment.Alipay != nil && cfg.Payment.Alipay != nil {
			if payment.Alipay.PrivateKey == "" || strings.HasPrefix(payment.Alipay.PrivateKey, "****") {
				payment.Alipay.PrivateKey = cfg.Payment.Alipay.PrivateKey
			}
			if payment.Alipay.AlipayPubKey == "" || strings.HasPrefix(payment.Alipay.AlipayPubKey, "****") {
				payment.Alipay.AlipayPubKey = cfg.Payment.Alipay.AlipayPubKey
			}
		}
		if payment.Wechat != nil && cfg.Payment.Wechat != nil {
			if payment.Wechat.APIv3Key == "" || strings.HasPrefix(payment.Wechat.APIv3Key, "****") {
				payment.Wechat.APIv3Key = cfg.Payment.Wechat.APIv3Key
			}
			if payment.Wechat.PrivateKey == "" || strings.HasPrefix(payment.Wechat.PrivateKey, "****") {
				payment.Wechat.PrivateKey = cfg.Payment.Wechat.PrivateKey
			}
		}
		if payment.Yungouos != nil && cfg.Payment != nil && cfg.Payment.Yungouos != nil {
			if payment.Yungouos.Wechat != nil && cfg.Payment.Yungouos.Wechat != nil &&
				(payment.Yungouos.Wechat.Key == "" || strings.HasPrefix(payment.Yungouos.Wechat.Key, "****")) {
				payment.Yungouos.Wechat.Key = cfg.Payment.Yungouos.Wechat.Key
			}
			if payment.Yungouos.Alipay != nil && cfg.Payment.Yungouos.Alipay != nil &&
				(payment.Yungouos.Alipay.Key == "" || strings.HasPrefix(payment.Yungouos.Alipay.Key, "****")) {
				payment.Yungouos.Alipay.Key = cfg.Payment.Yungouos.Alipay.Key
			}
		}
	}
	energy := req.Energy
	if energy == nil {
		energy = cfg.Energy
	}
	if err := config.SaveAdminConfig(h.configPath, channels, smtp, payment, energy); err != nil {
		log.Printf("[admin] SaveAdminConfig failed: path=%s err=%v", h.configPath, err)
		msg := err.Error()
		if len(msg) > 80 {
			msg = msg[:80] + "..."
		}
		// 简单转义避免破坏 JSON
		msg = strings.ReplaceAll(strings.ReplaceAll(msg, `\`, `\\`), `"`, `\"`)
		http.Error(w, `{"error":"failed to save config: `+msg+`"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

// TestChannelRequest 测试渠道/模型连通性
type TestChannelRequest struct {
	ChannelID  string `json:"channel_id"`  // 从已保存配置查找
	Model      string `json:"model"`       // 模型名
	APIBase string `json:"api_base"` // 或直接传凭证
	APIKey  string `json:"api_key"`
}

func (h *Handler) TestChannel(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req TestChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	apiBase, apiKey, model := "", "", ""
	if req.APIBase != "" && req.APIKey != "" {
		apiBase = strings.TrimSuffix(strings.TrimSpace(req.APIBase), "/")
		if apiBase == "" {
			apiBase = "https://api.openai.com/v1"
		}
		apiKey = strings.TrimSpace(req.APIKey)
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = "gpt-4o"
		}
	} else if req.ChannelID != "" && req.Model != "" {
		cfg, err := config.Load(h.configPath)
		if err != nil {
			http.Error(w, `{"error":"failed to load config"}`, http.StatusInternalServerError)
			return
		}
		model = strings.TrimSpace(req.Model)
		for _, ch := range cfg.Channels {
			if ch.ID != req.ChannelID {
				continue
			}
			apiKey = ch.APIKey
			apiBase = ch.APIBase
			if apiBase == "" {
				apiBase = "https://api.openai.com/v1"
			}
			apiBase = strings.TrimSuffix(apiBase, "/")
			break
		}
		if apiKey == "" {
			http.Error(w, `{"error":"channel not found or no api key"}`, http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, `{"error":"provide channel_id+model or api_base+api_key+model"}`, http.StatusBadRequest)
		return
	}

	reqURL := apiBase + "/chat/completions"
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "user", "content": "hi"},
		},
		"max_tokens": 5,
	}
	bodyBytes, _ := json.Marshal(body)
	proxyReq, err := http.NewRequestWithContext(r.Context(), "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	proxyReq.Header.Set("Content-Type", "application/json")
	proxyReq.Header.Set("Authorization", "Bearer "+apiKey)
	if u, err := url.Parse(reqURL); err == nil && u.Host != "" {
		host := u.Hostname()
		if p := u.Port(); p != "" && p != "443" && p != "80" {
			host = u.Host
		}
		proxyReq.Host = host
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		log.Printf("[admin] config test failed: url=%s model=%s err=%v", reqURL, model, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"message": "连接失败: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[admin] config test non-200: url=%s model=%s status=%d body=%s", reqURL, model, resp.StatusCode, string(respBody))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok":      false,
			"message": "上游返回 " + http.StatusText(resp.StatusCode) + ": " + string(respBody),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      true,
		"message": "连接正常",
	})
}

// TestSMTPRequest 测试 SMTP 连通性
type TestSMTPRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	User string `json:"user"`
	Pass string `json:"pass"`
	From string `json:"from"`
}

func (h *Handler) TestSMTP(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var req TestSMTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.Host == "" {
		cfg, err := config.Load(h.configPath)
		if err != nil || cfg.SMTP == nil || cfg.SMTP.Host == "" {
			http.Error(w, `{"error":"SMTP not configured"}`, http.StatusBadRequest)
			return
		}
		req.Host = cfg.SMTP.Host
		req.Port = cfg.SMTP.Port
		req.User = cfg.SMTP.User
		req.Pass = cfg.SMTP.Pass
		req.From = cfg.SMTP.From
	}
	if req.Port <= 0 {
		req.Port = 587
	}
	if req.From == "" {
		req.From = req.User
	}
	err := mail.TestSMTP(req.Host, req.Port, req.User, req.Pass, req.From)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		msg := err.Error()
		if strings.Contains(strings.ToLower(msg), "eof") {
			msg += "（163/QQ 等邮箱须使用授权码而非登录密码；发件人建议填邮箱账号）"
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": msg})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "连接正常"})
}
