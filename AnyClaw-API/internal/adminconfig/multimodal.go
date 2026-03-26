package adminconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"strings"
)

// multimodalTestPNGDataURL returns a solid-color PNG as an OpenAI-style data URL.
// Some upstreams (e.g. 讯飞 / One API 聚合) reject 1×1 probes as "resolution over limit";
// 256×256 keeps payload small while satisfying common minimums.
func multimodalTestPNGDataURL() (string, error) {
	const side = 256
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	fill := color.RGBA{R: 220, G: 20, B: 60, A: 255}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", fmt.Errorf("encode probe png: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

func applyRequestHost(req *http.Request, requestURL string) {
	u, err := url.Parse(requestURL)
	if err != nil || u.Host == "" {
		return
	}
	host := u.Hostname()
	if p := u.Port(); p != "" && p != "443" && p != "80" {
		host = u.Host
	}
	req.Host = host
}

func buildMultimodalImageChatBody(model string) ([]byte, error) {
	dataURL, err := multimodalTestPNGDataURL()
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "You are running an automated admin connectivity check. If you can see any image attached, reply with exactly: MULTIMODAL_IMAGE_OK. If you cannot see an image, reply with exactly: MULTIMODAL_IMAGE_FAIL.",
					},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": dataURL,
						},
					},
				},
			},
		},
		"max_tokens": 64,
	}
	return json.Marshal(body)
}

func extractFirstAssistantText(respBody []byte) string {
	var top map[string]any
	if err := json.Unmarshal(respBody, &top); err != nil {
		return ""
	}
	choices, _ := top["choices"].([]any)
	if len(choices) == 0 {
		return ""
	}
	c0, _ := choices[0].(map[string]any)
	msg, _ := c0["message"].(map[string]any)
	if s, ok := msg["content"].(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}
