package payment

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
)

const yungouosAPIBase = "https://api.pay.yungouos.com"

// yungouosSign 生成 YunGouOS 签名：参数按 key 升序，k1=v1&k2=v2&...&key=KEY，MD5 大写
func yungouosSign(params map[string]string, key string) string {
	var keys []string
	for k := range params {
		if k != "sign" && params[k] != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var buf strings.Builder
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte('&')
		}
		buf.WriteString(k)
		buf.WriteByte('=')
		buf.WriteString(params[k])
	}
	buf.WriteString("&key=")
	buf.WriteString(key)
	h := md5.Sum([]byte(buf.String()))
	return strings.ToUpper(hex.EncodeToString(h[:]))
}

// CreateYungouosWechatNativePay 微信扫码支付，返回 code_url（用于生成二维码）
func CreateYungouosWechatNativePay(cfg *config.YungouosChannel, notifyURL, outTradeNo, body string, totalCny int) (codeURL string, err error) {
	if cfg == nil || !cfg.Enabled || cfg.MchID == "" || cfg.Key == "" {
		return "", fmt.Errorf("yungouos wechat not configured")
	}
	totalYuan := fmt.Sprintf("%.2f", float64(totalCny)/100)
	params := map[string]string{
		"out_trade_no": outTradeNo,
		"total_fee":    totalYuan,
		"mch_id":       cfg.MchID,
		"body":         body,
		"type":         "1", // 1=返回原生支付链接，需自行生成二维码
	}
	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}
	params["sign"] = yungouosSign(params, cfg.Key)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	resp, err := http.Post(yungouosAPIBase+"/api/pay/wxpay/nativePay", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	bodyStr := buf.String()
	bodyStr = strings.TrimSpace(bodyStr)
	if strings.HasPrefix(bodyStr, "{") {
		var m map[string]any
		if json.Unmarshal([]byte(bodyStr), &m) == nil {
			if v, ok := m["code_url"].(string); ok && v != "" {
				codeURL = v
			} else if v, ok := m["data"].(string); ok && v != "" {
				codeURL = v
			} else if code, _ := m["code"].(float64); code != 0 {
				msg, _ := m["msg"].(string)
				return "", fmt.Errorf("yungouos wechat: %s", msg)
			}
		}
	} else {
		codeURL = bodyStr
	}
	if codeURL == "" {
		return "", fmt.Errorf("yungouos wechat nativePay no code_url: %s", bodyStr)
	}
	return codeURL, nil
}

// CreateYungouosAlipayNativePay 支付宝扫码支付，返回 code_url
func CreateYungouosAlipayNativePay(cfg *config.YungouosChannel, notifyURL, outTradeNo, body string, totalCny int) (codeURL string, err error) {
	if cfg == nil || !cfg.Enabled || cfg.MchID == "" || cfg.Key == "" {
		return "", fmt.Errorf("yungouos alipay not configured")
	}
	totalYuan := fmt.Sprintf("%.2f", float64(totalCny)/100)
	params := map[string]string{
		"out_trade_no": outTradeNo,
		"total_fee":    totalYuan,
		"mch_id":       cfg.MchID,
		"body":         body,
		"type":         "1",
	}
	if notifyURL != "" {
		params["notify_url"] = notifyURL
	}
	params["sign"] = yungouosSign(params, cfg.Key)

	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	resp, err := http.Post(yungouosAPIBase+"/api/pay/alipay/nativePay", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	bodyStr := buf.String()
	bodyStr = strings.TrimSpace(bodyStr)
	if strings.HasPrefix(bodyStr, "{") {
		var m map[string]any
		if json.Unmarshal([]byte(bodyStr), &m) == nil {
			if v, ok := m["code_url"].(string); ok && v != "" {
				codeURL = v
			} else if v, ok := m["data"].(string); ok && v != "" {
				codeURL = v
			} else if code, _ := m["code"].(float64); code != 0 {
				msg, _ := m["msg"].(string)
				return "", fmt.Errorf("yungouos alipay: %s", msg)
			}
		}
	} else {
		codeURL = bodyStr
	}
	if codeURL == "" {
		return "", fmt.Errorf("yungouos alipay nativePay no code_url: %s", bodyStr)
	}
	return codeURL, nil
}

// VerifyYungouosNotify 验证 YunGouOS 异步通知签名，返回 out_trade_no, pay_no, total_fee(分)
func VerifyYungouosNotify(mchID, key string, r *http.Request) (outTradeNo, payNo string, totalFee int, err error) {
	if err = r.ParseForm(); err != nil {
		return "", "", 0, err
	}
	params := make(map[string]string)
	for k, v := range r.Form {
		if len(v) > 0 {
			params[k] = v[0]
		}
	}
	gotSign := params["sign"]
	if gotSign == "" {
		return "", "", 0, fmt.Errorf("no sign in notify")
	}
	delete(params, "sign")
	expectSign := yungouosSign(params, key)
	if gotSign != expectSign {
		return "", "", 0, fmt.Errorf("yungouos notify sign mismatch")
	}
	outTradeNo = params["out_trade_no"]
	payNo = params["pay_no"]
	if payNo == "" {
		payNo = params["transaction_id"]
	}
	totalStr := params["total_fee"]
	if totalStr != "" {
		if f, e := strconv.ParseFloat(totalStr, 64); e == nil {
			totalFee = int(f * 100)
		}
	}
	return outTradeNo, payNo, totalFee, nil
}
