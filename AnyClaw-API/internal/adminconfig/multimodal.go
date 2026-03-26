package adminconfig

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "embed"
)

//go:embed testdata/probe.mp4
var embeddedProbeMP4 []byte

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

func moonshotLikeAPIBase(apiBase string) bool {
	lb := strings.ToLower(strings.TrimSpace(apiBase))
	return strings.Contains(lb, "moonshot")
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

func moonshotUploadVideoFile(ctx context.Context, apiBase, apiKey string) (fileID string, err error) {
	if len(embeddedProbeMP4) == 0 {
		return "", fmt.Errorf("embedded probe video is empty (rebuild with testdata/probe.mp4)")
	}
	base := strings.TrimSuffix(strings.TrimSpace(apiBase), "/")
	uploadURL := base + "/files"

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("purpose", "video"); err != nil {
		return "", err
	}
	part, err := mw.CreateFormFile("file", "anyclaw_admin_probe.mp4")
	if err != nil {
		return "", err
	}
	if _, err := part.Write(embeddedProbeMP4); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	applyRequestHost(req, uploadURL)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("files API %s: %s", resp.Status, truncateForLog(string(respBody), 800))
	}
	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil || parsed.ID == "" {
		return "", fmt.Errorf("files API: no file id in response: %s", truncateForLog(string(respBody), 400))
	}
	return parsed.ID, nil
}

func buildMoonshotVideoChatBody(model, fileID string) ([]byte, error) {
	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "video_url",
						"video_url": map[string]any{
							"url": "ms://" + fileID,
						},
					},
					map[string]any{
						"type": "text",
						"text": "Admin probe: reply with one short phrase describing the video (or say BLANK if you see no motion).",
					},
				},
			},
		},
		"max_tokens": 128,
	}
	return json.Marshal(body)
}

// buildInlineMP4VideoChatBody sends the embedded probe clip as data:video/mp4;base64,... in one chat/completions
// request. Used for OpenAI-compatible / One API gateways that do not support Moonshot ms:// or /v1/files routing.
// The model parameter is the channel model (e.g. astron-code-latest).
func buildInlineMP4VideoChatBody(model string) ([]byte, error) {
	if len(embeddedProbeMP4) == 0 {
		return nil, fmt.Errorf("embedded probe video is empty")
	}
	dataURL := "data:video/mp4;base64," + base64.StdEncoding.EncodeToString(embeddedProbeMP4)
	body := map[string]any{
		"model": model,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{
						"type": "video_url",
						"video_url": map[string]any{
							"url": dataURL,
						},
					},
					map[string]any{
						"type": "text",
						"text": "Admin probe: reply with one short phrase describing the video (or say BLANK if you cannot process video).",
					},
				},
			},
		},
		"max_tokens": 128,
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
