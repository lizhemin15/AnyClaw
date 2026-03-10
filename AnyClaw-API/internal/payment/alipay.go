package payment

import (
	"fmt"
	"net/http"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/smartwalle/alipay/v3"
)

// CreateAlipayPagePay 创建电脑网站支付，返回支付 URL
func CreateAlipayPagePay(cfg *config.AlipayConfig, notifyURL, returnURL, outTradeNo, subject string, totalCny int) (string, error) {
	if cfg == nil || !cfg.Enabled || cfg.AppID == "" || cfg.PrivateKey == "" {
		return "", fmt.Errorf("alipay not configured")
	}
	client, err := alipay.New(cfg.AppID, cfg.PrivateKey, !cfg.IsSandbox)
	if err != nil {
		return "", err
	}
	if cfg.AlipayPubKey != "" {
		if err = client.LoadAliPayPublicKey(cfg.AlipayPubKey); err != nil {
			return "", err
		}
	}
	amount := fmt.Sprintf("%.2f", float64(totalCny)/100)
	pay := alipay.TradePagePay{}
	pay.Trade = alipay.Trade{
		Subject:     subject,
		OutTradeNo:  outTradeNo,
		TotalAmount: amount,
		ProductCode: "FAST_INSTANT_TRADE_PAY",
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
	}
	return client.TradePagePay(pay)
}

// VerifyAlipayNotify 验证并解析支付宝异步通知
func VerifyAlipayNotify(cfg *config.AlipayConfig, r *http.Request) (outTradeNo, tradeNo string, totalAmount int, err error) {
	if cfg == nil || cfg.PrivateKey == "" {
		err = fmt.Errorf("alipay not configured")
		return
	}
	client, err := alipay.New(cfg.AppID, cfg.PrivateKey, !cfg.IsSandbox)
	if err != nil {
		return
	}
	if cfg.AlipayPubKey != "" {
		if err = client.LoadAliPayPublicKey(cfg.AlipayPubKey); err != nil {
			return
		}
	}
	noti, err := client.DecodeNotification(r.Form)
	if err != nil {
		return
	}
	if noti.TradeStatus != alipay.TradeStatusSuccess {
		err = fmt.Errorf("trade status not success: %s", noti.TradeStatus)
		return
	}
	outTradeNo = noti.OutTradeNo
	tradeNo = noti.TradeNo
	var f float64
	if _, e := fmt.Sscanf(noti.TotalAmount, "%f", &f); e == nil {
		totalAmount = int(f * 100)
	}
	return
}

// ACKAlipay 回复支付宝已收到通知
func ACKAlipay(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success"))
}

