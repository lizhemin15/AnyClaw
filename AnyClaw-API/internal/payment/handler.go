package payment

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/google/uuid"
)

type Handler struct {
	configPath string
	db         *db.DB
	apiBaseURL string
}

func New(configPath string, database *db.DB, apiBaseURL string) *Handler {
	return &Handler{configPath: configPath, db: database, apiBaseURL: apiBaseURL}
}

// 固定三档充值档位默认值
var defaultPlans = []config.PaymentPlan{
	{ID: "plan-1", Name: "入门", Energy: 100, PriceCny: 100, Sort: 0},
	{ID: "plan-2", Name: "进阶", Energy: 500, PriceCny: 450, Sort: 1},
	{ID: "plan-3", Name: "尊享", Energy: 2000, PriceCny: 1600, Sort: 2},
}

// GetPlans 获取充值档位（固定三档，至少有一个支付渠道启用时返回）
func (h *Handler) GetPlans(w http.ResponseWriter, r *http.Request) {
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	yg := cfg.Payment.Yungouos
	hasChannel := yg != nil && ((yg.Alipay != nil && yg.Alipay.Enabled) || (yg.Wechat != nil && yg.Wechat.Enabled))
	if !hasChannel {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]any{})
		return
	}
	plans := cfg.Payment.Plans
	if plans == nil {
		plans = []config.PaymentPlan{}
	}
	// 固定三档：取配置前 3 个，不足则用默认值补齐
	out := make([]config.PaymentPlan, 3)
	for i := 0; i < 3; i++ {
		if i < len(plans) {
			out[i] = plans[i]
			out[i].ID = defaultPlans[i].ID
			out[i].Sort = i
		} else {
			out[i] = defaultPlans[i]
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

// ListOrders 获取订单列表：普通用户仅自己的，管理员可看全部（含用户邮箱）
func (h *Handler) ListOrders(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if claims.Role == "admin" {
		list, err := h.db.ListOrdersAll(100)
		if err != nil {
			log.Printf("[payment] list orders all failed: %v", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
		return
	}
	list, err := h.db.ListOrdersForUser(claims.UserID, 50)
	if err != nil {
		log.Printf("[payment] list orders failed: %v", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// CreateOrderRequest 创建订单请求
type CreateOrderRequest struct {
	PlanID  string `json:"plan_id"`
	Channel string `json:"channel"` // alipay | wechat
}

// CreateOrder 创建支付订单，返回支付 URL 或 code_url
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.PlanID = strings.TrimSpace(req.PlanID)
	req.Channel = strings.TrimSpace(strings.ToLower(req.Channel))
	if req.PlanID == "" || (req.Channel != "alipay" && req.Channel != "wechat") {
		http.Error(w, `{"error":"plan_id and channel (alipay|wechat) required"}`, http.StatusBadRequest)
		return
	}

	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil {
		http.Error(w, `{"error":"payment not configured"}`, http.StatusInternalServerError)
		return
	}

	var plan *config.PaymentPlan
	for i := range cfg.Payment.Plans {
		if cfg.Payment.Plans[i].ID == req.PlanID {
			plan = &cfg.Payment.Plans[i]
			break
		}
	}
	if plan == nil || plan.Energy <= 0 || plan.PriceCny <= 0 {
		http.Error(w, `{"error":"invalid plan"}`, http.StatusBadRequest)
		return
	}

	outTradeNo := "ord-" + uuid.New().String()
	_, err = h.db.CreateOrder(claims.UserID, plan.ID, plan.Energy, plan.PriceCny, req.Channel, outTradeNo)
	if err != nil {
		log.Printf("[payment] create order failed: %v", err)
		http.Error(w, `{"error":"failed to create order"}`, http.StatusInternalServerError)
		return
	}

	baseURL := strings.TrimSuffix(h.apiBaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	notifyURL := baseURL + "/api/payment/notify/" + req.Channel

	subject := "AnyClaw 金币充值 - " + plan.Name
	yg := cfg.Payment.Yungouos

	switch req.Channel {
	case "alipay":
		if yg != nil && yg.Alipay != nil && yg.Alipay.Enabled {
			codeURL, err := CreateYungouosAlipayNativePay(yg.Alipay, notifyURL, outTradeNo, subject, plan.PriceCny)
			if err != nil {
				log.Printf("[payment] yungouos alipay create failed: %v", err)
				http.Error(w, `{"error":"alipay failed: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"out_trade_no": outTradeNo, "code_url": codeURL})
		} else {
			http.Error(w, `{"error":"alipay not enabled (请配置 YunGouOS 支付宝)"}`, http.StatusBadRequest)
		}
	case "wechat":
		if yg != nil && yg.Wechat != nil && yg.Wechat.Enabled {
			codeURL, err := CreateYungouosWechatNativePay(yg.Wechat, notifyURL, outTradeNo, subject, plan.PriceCny)
			if err != nil {
				log.Printf("[payment] yungouos wechat create failed: %v", err)
				http.Error(w, `{"error":"wechat failed: `+err.Error()+`"}`, http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"out_trade_no": outTradeNo, "code_url": codeURL})
		} else {
			http.Error(w, `{"error":"wechat not enabled (请配置 YunGouOS 微信)"}`, http.StatusBadRequest)
		}
	default:
		http.Error(w, `{"error":"unsupported channel"}`, http.StatusBadRequest)
	}
}

// NotifyAlipay YunGouOS 支付宝异步通知
func (h *Handler) NotifyAlipay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SUCCESS"))
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil || cfg.Payment.Yungouos == nil || cfg.Payment.Yungouos.Alipay == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SUCCESS"))
		return
	}
	yg := cfg.Payment.Yungouos.Alipay
	outTradeNo, tradeNo, totalAmount, err := VerifyYungouosNotify(yg.MchID, yg.Key, r)
	if err != nil {
		log.Printf("[payment] yungouos alipay notify verify failed: %v", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SUCCESS"))
		return
	}
	ok, err := h.db.MarkOrderPaid(outTradeNo, tradeNo)
	if err != nil {
		log.Printf("[payment] alipay mark paid failed: %v", err)
	}
	if ok {
		ord, _ := h.db.GetOrderByOutTradeNo(outTradeNo)
		if ord != nil && totalAmount >= ord.PriceCny {
			_ = h.db.AddUserEnergy(ord.UserID, ord.Energy)
			log.Printf("[payment] yungouos alipay order %s paid, user %d +%d energy", outTradeNo, ord.UserID, ord.Energy)
			if inviterID, has := h.db.GetUserInviterID(ord.UserID); has && inviterID > 0 {
				cfg, _ := config.Load(h.configPath)
				rate := config.GetEnergyConfig(cfg).InviteCommissionRate
				if rate > 0 && rate <= 100 {
					commission := ord.Energy * rate / 100
					if commission > 0 {
						_ = h.db.AddUserEnergy(inviterID, commission)
						log.Printf("[payment] invite commission: inviter %d +%d (%.0f%%)", inviterID, commission, float64(rate))
					}
				}
			}
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("SUCCESS"))
}

// NotifyWechat YunGouOS 微信支付异步通知
func (h *Handler) NotifyWechat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SUCCESS"))
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil || cfg.Payment.Yungouos == nil || cfg.Payment.Yungouos.Wechat == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("SUCCESS"))
		return
	}
	yg := cfg.Payment.Yungouos.Wechat
	outTradeNo, tradeNo, totalAmount, err := VerifyYungouosNotify(yg.MchID, yg.Key, r)
	if err != nil {
		log.Printf("[payment] yungouos wechat notify verify failed: %v", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("FAIL"))
		return
	}
	ok, err := h.db.MarkOrderPaid(outTradeNo, tradeNo)
	if err != nil {
		log.Printf("[payment] yungouos wechat mark paid failed: %v", err)
	}
	if ok {
		ord, _ := h.db.GetOrderByOutTradeNo(outTradeNo)
		if ord != nil && totalAmount >= ord.PriceCny {
			_ = h.db.AddUserEnergy(ord.UserID, ord.Energy)
			log.Printf("[payment] yungouos wechat order %s paid, user %d +%d energy", outTradeNo, ord.UserID, ord.Energy)
			if inviterID, has := h.db.GetUserInviterID(ord.UserID); has && inviterID > 0 {
				rate := config.GetEnergyConfig(cfg).InviteCommissionRate
				if rate > 0 && rate <= 100 {
					commission := ord.Energy * rate / 100
					if commission > 0 {
						_ = h.db.AddUserEnergy(inviterID, commission)
						log.Printf("[payment] invite commission: inviter %d +%d (%.0f%%)", inviterID, commission, float64(rate))
					}
				}
			}
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("SUCCESS"))
}
