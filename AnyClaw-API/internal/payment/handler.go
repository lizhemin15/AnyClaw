package payment

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/google/uuid"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
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
	hasChannel := (cfg.Payment.Alipay != nil && cfg.Payment.Alipay.Enabled) ||
		(cfg.Payment.Wechat != nil && cfg.Payment.Wechat.Enabled)
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
	returnURL := baseURL + "/recharge?paid=1"

	subject := "AnyClaw 金币充值 - " + plan.Name

	switch req.Channel {
	case "alipay":
		if cfg.Payment.Alipay == nil || !cfg.Payment.Alipay.Enabled {
			http.Error(w, `{"error":"alipay not enabled"}`, http.StatusBadRequest)
			return
		}
		payURL, err := CreateAlipayPagePay(cfg.Payment.Alipay, notifyURL, returnURL, outTradeNo, subject, plan.PriceCny)
		if err != nil {
			log.Printf("[payment] alipay create failed: %v", err)
			http.Error(w, `{"error":"alipay failed: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"out_trade_no": outTradeNo, "pay_url": payURL})
	case "wechat":
		if cfg.Payment.Wechat == nil || !cfg.Payment.Wechat.Enabled {
			http.Error(w, `{"error":"wechat not enabled"}`, http.StatusBadRequest)
			return
		}
		codeURL, err := CreateWechatNativePay(cfg.Payment.Wechat, notifyURL, outTradeNo, subject, plan.PriceCny)
		if err != nil {
			log.Printf("[payment] wechat create failed: %v", err)
			http.Error(w, `{"error":"wechat failed: `+err.Error()+`"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"out_trade_no": outTradeNo, "code_url": codeURL})
	default:
		http.Error(w, `{"error":"unsupported channel"}`, http.StatusBadRequest)
	}
}

// NotifyAlipay 支付宝异步通知
func (h *Handler) NotifyAlipay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		ACKAlipay(w)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil || cfg.Payment.Alipay == nil {
		ACKAlipay(w)
		return
	}
	outTradeNo, tradeNo, totalAmount, err := VerifyAlipayNotify(cfg.Payment.Alipay, r)
	if err != nil {
		log.Printf("[payment] alipay notify verify failed: %v", err)
		ACKAlipay(w)
		return
	}
	ok, err := h.db.MarkOrderPaid(outTradeNo, tradeNo)
	if err != nil {
		log.Printf("[payment] alipay mark paid failed: %v", err)
		ACKAlipay(w)
		return
	}
	if ok {
		ord, _ := h.db.GetOrderByOutTradeNo(outTradeNo)
		if ord != nil && totalAmount >= ord.PriceCny {
			_ = h.db.AddUserEnergy(ord.UserID, ord.Energy)
			log.Printf("[payment] alipay order %s paid, user %d +%d energy", outTradeNo, ord.UserID, ord.Energy)
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
	ACKAlipay(w)
}

// NotifyWechat 微信支付异步通知
func (h *Handler) NotifyWechat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, err := config.Load(h.configPath)
	if err != nil || cfg.Payment == nil || cfg.Payment.Wechat == nil {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
		return
	}
	wc := cfg.Payment.Wechat
	key, err := parseWechatPrivateKey(wc.PrivateKey)
	if err != nil {
		log.Printf("[payment] wechat notify: parse key failed: %v", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
		return
	}
	ctx := context.Background()
	if err := downloader.MgrInstance().RegisterDownloaderWithPrivateKey(ctx, key, wc.SerialNo, wc.MchID, wc.APIv3Key); err != nil {
		log.Printf("[payment] wechat notify: register downloader failed: %v", err)
	}
	certVisitor := downloader.MgrInstance().GetCertificateVisitor(wc.MchID)
	nh := notify.NewNotifyHandler(wc.APIv3Key, verifiers.NewSHA256WithRSAVerifier(certVisitor))
	transaction := new(payments.Transaction)
	_, err = nh.ParseNotifyRequest(ctx, r, transaction)
	if err != nil {
		log.Printf("[payment] wechat notify verify failed: %v", err)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
		return
	}
	outTradeNo := ""
	if transaction.OutTradeNo != nil {
		outTradeNo = *transaction.OutTradeNo
	}
	tradeNo := ""
	if transaction.TransactionId != nil {
		tradeNo = *transaction.TransactionId
	}
	tradeState := ""
	if transaction.TradeState != nil {
		tradeState = *transaction.TradeState
	}
	if outTradeNo == "" || tradeState != "SUCCESS" {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
		return
	}
	ok, err := h.db.MarkOrderPaid(outTradeNo, tradeNo)
	if err != nil {
		log.Printf("[payment] wechat mark paid failed: %v", err)
	}
	if ok {
		ord, _ := h.db.GetOrderByOutTradeNo(outTradeNo)
		if ord != nil {
			_ = h.db.AddUserEnergy(ord.UserID, ord.Energy)
			log.Printf("[payment] wechat order %s paid, user %d +%d energy", outTradeNo, ord.UserID, ord.Energy)
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
	w.Write([]byte(`{"code":"SUCCESS","message":"成功"}`))
}
