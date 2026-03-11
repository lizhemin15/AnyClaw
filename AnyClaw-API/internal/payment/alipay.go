package payment

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/smartwalle/alipay/v3"
)

func newAlipayClient(cfg *config.AlipayConfig) (*alipay.Client, error) {
	if cfg == nil || !cfg.Enabled || cfg.AppID == "" || cfg.PrivateKey == "" {
		return nil, fmt.Errorf("alipay not configured")
	}
	client, err := alipay.New(cfg.AppID, cfg.PrivateKey, !cfg.IsSandbox)
	if err != nil {
		return nil, err
	}
	if cfg.AlipayPubKey != "" {
		if err = client.LoadAliPayPublicKey(cfg.AlipayPubKey); err != nil {
			return nil, err
		}
	}
	return client, nil
}

// CreateAlipayPagePay 创建电脑网站支付，返回支付 URL
func CreateAlipayPagePay(cfg *config.AlipayConfig, notifyURL, returnURL, outTradeNo, subject string, totalCny int) (string, error) {
	client, err := newAlipayClient(cfg)
	if err != nil {
		return "", err
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
	u, err := client.TradePagePay(pay)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// CreateAlipayWapPay 创建手机网站支付，返回支付 URL（手机端可唤起支付宝 APP）
func CreateAlipayWapPay(cfg *config.AlipayConfig, notifyURL, returnURL, outTradeNo, subject string, totalCny int) (string, error) {
	client, err := newAlipayClient(cfg)
	if err != nil {
		return "", err
	}
	amount := fmt.Sprintf("%.2f", float64(totalCny)/100)
	pay := alipay.TradeWapPay{}
	pay.Trade = alipay.Trade{
		Subject:     subject,
		OutTradeNo:  outTradeNo,
		TotalAmount: amount,
		ProductCode: "QUICK_WAP_WAY",
		NotifyURL:   notifyURL,
		ReturnURL:   returnURL,
	}
	u, err := client.TradeWapPay(pay)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// CreateAlipayPreCreate 当面付扫码支付（alipay.trade.precreate），返回二维码内容
func CreateAlipayPreCreate(cfg *config.AlipayConfig, notifyURL, outTradeNo, subject string, totalCny int) (string, error) {
	client, err := newAlipayClient(cfg)
	if err != nil {
		return "", err
	}
	amount := fmt.Sprintf("%.2f", float64(totalCny)/100)
	pay := alipay.TradePreCreate{}
	pay.Trade = alipay.Trade{
		Subject:     subject,
		OutTradeNo:  outTradeNo,
		TotalAmount: amount,
		ProductCode: "FACE_TO_FACE_PAYMENT",
		NotifyURL:   notifyURL,
	}
	rsp, err := client.TradePreCreate(context.Background(), pay)
	if err != nil {
		return "", err
	}
	if rsp == nil || rsp.QRCode == "" {
		return "", fmt.Errorf("alipay precreate no qr_code")
	}
	return rsp.QRCode, nil
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
	noti, err := client.DecodeNotification(context.Background(), r.Form)
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

