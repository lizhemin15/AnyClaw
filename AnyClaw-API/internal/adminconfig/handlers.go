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
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/mail"
	"github.com/anyclaw/anyclaw-api/internal/request"
)

type Handler struct {
	configPath string
	database   *db.DB
}

func New(configPath string, database *db.DB) *Handler {
	return &Handler{configPath: configPath, database: database}
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
			"id":                 ch.ID,
			"name":               ch.Name,
			"api_key":            config.MaskAPIKey(ch.APIKey),
			"api_base":           ch.APIBase,
			"enabled":            ch.Enabled,
			"models":             ch.Models,
			"daily_tokens_limit": ch.DailyTokensLimit,
			"qps_limit":          ch.QPSLimit,
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
	if cfg.Container != nil {
		resp["container"] = cfg.Container
	} else {
		resp["container"] = map[string]any{"workspace_size_gb": 0}
	}
	if cfg.APIURL != "" {
		resp["api_url"] = cfg.APIURL
	}
	if cfg.COS != nil {
		resp["cos"] = map[string]any{
			"enabled":     cfg.COS.Enabled,
			"secret_id":  config.MaskAPIKey(cfg.COS.SecretID),
			"secret_key": config.MaskAPIKey(cfg.COS.SecretKey),
			"bucket":     cfg.COS.Bucket,
			"region":     cfg.COS.Region,
			"domain":     cfg.COS.Domain,
			"path_prefix": cfg.COS.PathPrefix,
		}
	} else {
		resp["cos"] = map[string]any{"enabled": false, "bucket": "", "region": "", "domain": "", "path_prefix": "media/"}
	}
	anyclawAPI := cfg.AnyclawAPI
	if anyclawAPI == nil {
		anyclawAPI = []config.AnyclawAPIEndpoint{}
	}
	maskedAPI := make([]map[string]any, len(anyclawAPI))
	for i, ep := range anyclawAPI {
		maskedAPI[i] = map[string]any{
			"id":                 ep.ID,
			"name":               ep.Name,
			"endpoint":           ep.Endpoint,
			"api_key":            config.MaskAPIKey(ep.APIKey),
			"enabled":            ep.Enabled,
			"daily_tokens_limit": ep.DailyTokensLimit,
			"qps_limit":          ep.QPSLimit,
		}
	}
	resp["anyclaw_api"] = maskedAPI
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
		Channels   []config.Channel              `json:"channels"`
		AnyclawAPI []config.AnyclawAPIEndpoint   `json:"anyclaw_api"`
		SMTP       *config.SMTPConfig            `json:"smtp"`
		Payment    *config.PaymentConfig         `json:"payment"`
		Energy     *config.EnergyConfig          `json:"energy"`
		Container  *config.ContainerConfig       `json:"container"`
		COS        *config.COSConfig             `json:"cos"`
		APIURL     string                        `json:"api_url"`
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
	// Merge AnyclawAPI: preserve existing api_key if client sent masked value
	anyclawAPI := req.AnyclawAPI
	if anyclawAPI == nil {
		anyclawAPI = []config.AnyclawAPIEndpoint{}
	}
	existingAPI := make(map[string]string)
	for _, ep := range cfg.AnyclawAPI {
		existingAPI[ep.ID] = ep.APIKey
	}
	for i := range anyclawAPI {
		if k, ok := existingAPI[anyclawAPI[i].ID]; ok && (anyclawAPI[i].APIKey == "" || strings.HasPrefix(anyclawAPI[i].APIKey, "****")) {
			anyclawAPI[i].APIKey = k
		}
	}
	// Merge SMTP: preserve existing if not sent; clear if host empty; preserve pass if masked
	smtp := req.SMTP
	if smtp == nil {
		smtp = cfg.SMTP
	} else if strings.TrimSpace(smtp.Host) == "" {
		smtp = nil
	} else if cfg.SMTP != nil && (smtp.Pass == "" || strings.HasPrefix(smtp.Pass, "****")) {
		smtp.Pass = cfg.SMTP.Pass
	}
	// Merge Payment: preserve secrets if client sent masked value; keep existing if not sent
	payment := req.Payment
	if payment == nil {
		payment = cfg.Payment
	} else if cfg.Payment != nil && payment.Yungouos != nil && cfg.Payment.Yungouos != nil {
		if payment.Yungouos.Wechat != nil && cfg.Payment.Yungouos.Wechat != nil &&
			(payment.Yungouos.Wechat.Key == "" || strings.HasPrefix(payment.Yungouos.Wechat.Key, "****")) {
			payment.Yungouos.Wechat.Key = cfg.Payment.Yungouos.Wechat.Key
		}
		if payment.Yungouos.Alipay != nil && cfg.Payment.Yungouos.Alipay != nil &&
			(payment.Yungouos.Alipay.Key == "" || strings.HasPrefix(payment.Yungouos.Alipay.Key, "****")) {
			payment.Yungouos.Alipay.Key = cfg.Payment.Yungouos.Alipay.Key
		}
	}
	energy := req.Energy
	if energy == nil {
		energy = cfg.Energy
	}
	container := req.Container
	if container == nil {
		container = cfg.Container
	}
	if container != nil && container.WorkspaceSizeGB < 0 {
		container.WorkspaceSizeGB = 0
	}
	cos := req.COS
	if cos == nil {
		cos = cfg.COS
	} else {
		if cfg.COS != nil {
			if cos.SecretID == "" || strings.HasPrefix(cos.SecretID, "****") {
				cos.SecretID = cfg.COS.SecretID
			}
			if cos.SecretKey == "" || strings.HasPrefix(cos.SecretKey, "****") {
				cos.SecretKey = cfg.COS.SecretKey
			}
		}
		if !cos.Enabled {
			cos = nil
		}
	}
	apiURL := strings.TrimSpace(req.APIURL)
	// 全部写入数据库，DB 为唯一数据源
	dbPayload := map[string]any{"channels": channels, "anyclaw_api": anyclawAPI, "smtp": smtp, "payment": payment, "energy": energy, "container": container, "cos": cos, "api_url": apiURL}
	dbBytes, _ := json.Marshal(dbPayload)
	if h.database == nil {
		http.Error(w, `{"error":"database not configured"}`, http.StatusInternalServerError)
		return
	}
	if err := h.database.SaveAdminConfigJSON(dbBytes); err != nil {
		log.Printf("[admin] SaveAdminConfig to DB failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save config to database: " + err.Error()})
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
	// 前端保存后再次进入时，密码被脱敏为 ****，需从已保存配置取真实密码
	needFromConfig := req.Host == "" || req.Pass == "" || strings.HasPrefix(req.Pass, "****")
	if needFromConfig {
		cfg, err := config.Load(h.configPath)
		if err != nil || cfg.SMTP == nil || cfg.SMTP.Host == "" {
			http.Error(w, `{"error":"SMTP not configured"}`, http.StatusBadRequest)
			return
		}
		if req.Host == "" {
			req.Host = cfg.SMTP.Host
			req.Port = cfg.SMTP.Port
			req.User = cfg.SMTP.User
			req.Pass = cfg.SMTP.Pass
			req.From = cfg.SMTP.From
		} else if req.Pass == "" || strings.HasPrefix(req.Pass, "****") {
			req.Pass = cfg.SMTP.Pass
		}
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
		lower := strings.ToLower(msg)
		if strings.Contains(lower, "eof") {
			msg += "（163/QQ 等邮箱须使用授权码而非登录密码；发件人建议填邮箱账号）"
		} else if strings.Contains(lower, "535") || strings.Contains(lower, "authentication failed") {
			msg += "（请检查：1) 163/QQ/126 等须用授权码而非登录密码；2) 用户名填完整邮箱；3) 发件人需与登录账号一致；4) 邮箱设置中已开启 SMTP 服务）"
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": msg})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "连接正常"})
}
