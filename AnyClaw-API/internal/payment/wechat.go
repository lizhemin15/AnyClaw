package payment

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
)

func parseWechatPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("invalid private key pem")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	k, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key is not RSA")
	}
	return k, nil
}

// CreateWechatNativePay 创建微信 Native 扫码支付，返回 code_url（用于生成二维码）
func CreateWechatNativePay(cfg *config.WechatConfig, notifyURL, outTradeNo, description string, totalCny int) (codeURL string, err error) {
	if cfg == nil || !cfg.Enabled || cfg.AppID == "" || cfg.MchID == "" || cfg.APIv3Key == "" || cfg.PrivateKey == "" || cfg.SerialNo == "" {
		err = fmt.Errorf("wechat pay not configured")
		return
	}
	key, err := parseWechatPrivateKey(cfg.PrivateKey)
	if err != nil {
		return
	}
	opts := []core.ClientOption{
		option.WithWechatPayAutoAuthCipher(cfg.MchID, cfg.SerialNo, key, cfg.APIv3Key),
	}
	client, err := core.NewClient(context.Background(), opts...)
	if err != nil {
		return
	}
	svc := native.NativeApiService{Client: client}
	resp, result, err := svc.Prepay(context.Background(),
		native.PrepayRequest{
			Appid:       core.String(cfg.AppID),
			Mchid:       core.String(cfg.MchID),
			Description: core.String(description),
			OutTradeNo:  core.String(outTradeNo),
			NotifyUrl:   core.String(notifyURL),
			Amount: &native.Amount{
				Total: core.Int64(int64(totalCny)),
			},
		},
	)
	if err != nil {
		return
	}
	if result.Response.StatusCode != 200 {
		err = fmt.Errorf("wechat pay prepay failed: %d", result.Response.StatusCode)
		return
	}
	if resp.CodeUrl != nil {
		codeURL = *resp.CodeUrl
	}
	return
}

