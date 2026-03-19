package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
)

// Synthesizer converts text to speech audio.
type Synthesizer interface {
	Name() string
	Synthesize(ctx context.Context, text, voiceID string) (string, error) // returns local temp file path
}

// OpenAISynthesizer uses OpenAI TTS (tts-1) for speech synthesis.
type OpenAISynthesizer struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

func NewOpenAISynthesizer(apiKey, apiBase string) *OpenAISynthesizer {
	if apiBase == "" {
		apiBase = "https://api.chatanywhere.org/v1"
	}
	logger.DebugCF("voice", "Creating OpenAI synthesizer", map[string]any{
		"has_api_key": apiKey != "",
		"api_base":    apiBase,
	})
	return &OpenAISynthesizer{
		apiKey:  apiKey,
		apiBase: apiBase,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (s *OpenAISynthesizer) Name() string { return "openai" }

func (s *OpenAISynthesizer) Synthesize(ctx context.Context, text, voiceID string) (string, error) {
	if voiceID == "" {
		voiceID = "alloy"
	}
	logger.InfoCF("voice", "Starting TTS synthesis", map[string]any{
		"voice":       voiceID,
		"text_length": len(text),
	})

	bodyBytes, err := json.Marshal(map[string]string{
		"model": "tts-1",
		"input": text,
		"voice": voiceID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := s.apiBase + "/audio/speech"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(errBody))
	}

	tmpFile, err := os.CreateTemp("", "anyclaw_tts_*.mp3")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write audio data: %w", err)
	}

	logger.InfoCF("voice", "TTS synthesis completed", map[string]any{"path": tmpFile.Name()})
	return tmpFile.Name(), nil
}

// XiaomiMiMoSynthesizer uses Xiaomi MiMo TTS (platform.xiaomimimo.com) for speech synthesis.
// API: POST /v1/chat/completions with messages + audio, auth via api-key header.
// Docs: https://platform.xiaomimimo.com/#/docs/usage-guide/speech-synthesis
type XiaomiMiMoSynthesizer struct {
	apiKey     string
	apiBase    string
	model      string
	httpClient *http.Client
}

func NewXiaomiMiMoSynthesizer(apiKey, apiBase, model string) *XiaomiMiMoSynthesizer {
	if apiBase == "" {
		apiBase = "https://api.xiaomimimo.com/v1"
	}
	// platform.xiaomimimo.com 会返回 401+loginUrl，必须用 api 域名
	apiBase = strings.ReplaceAll(strings.ToLower(apiBase), "platform.xiaomimimo.com", "api.xiaomimimo.com")
	// 平台路径为 /v1/chat/completions，无 /api。若配置了 /api/v1 则修正
	apiBase = strings.ReplaceAll(strings.ReplaceAll(apiBase, "/api/v1", "/v1"), "/API/v1", "/v1")
	if model == "" {
		model = "mimo-v2-tts"
	}
	logger.DebugCF("voice", "Creating Xiaomi MiMo synthesizer", map[string]any{
		"has_api_key": apiKey != "",
		"api_base":    apiBase,
		"model":       model,
	})
	// 直连不走代理，避免容器内 HTTP_PROXY 导致 404
	tr := &http.Transport{Proxy: func(*http.Request) (*url.URL, error) { return nil, nil }}
	return &XiaomiMiMoSynthesizer{
		apiKey:  apiKey,
		apiBase: apiBase,
		model:   model,
		httpClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}
}

func (s *XiaomiMiMoSynthesizer) Name() string { return "xiaomi_mimo" }

func (s *XiaomiMiMoSynthesizer) Synthesize(ctx context.Context, text, voiceID string) (string, error) {
	if voiceID == "" {
		voiceID = "mimo_default"
	} else if voiceID == "default" {
		voiceID = "mimo_default"
	}
	logger.InfoCF("voice", "Starting Xiaomi MiMo TTS synthesis", map[string]any{
		"voice":       voiceID,
		"text_length": len(text),
	})

	// Xiaomi MiMo TTS: POST /v1/chat/completions with messages + audio, api-key header.
	body := map[string]any{
		"model": s.model,
		"messages": []map[string]string{
			{"role": "user", "content": "Please read the following text."},
			{"role": "assistant", "content": text},
		},
		"audio": map[string]string{
			"format": "wav",
			"voice":  voiceID,
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	reqURL := strings.TrimSuffix(s.apiBase, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", s.apiKey)
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		errMsg := string(respBody)
		if resp.StatusCode == 401 {
			// 避免返回 loginUrl 等误导 AI 说「token过期」「待授权」
			if strings.Contains(errMsg, "loginUrl") {
				errMsg = "API Key 无效或未配置，请在管理后台检查 TTS 的 API Key"
			}
		}
		return "", fmt.Errorf("API error (status %d) from %s: %s", resp.StatusCode, reqURL, errMsg)
	}

	var result struct {
		Choices []struct {
			Message struct {
				Audio *struct {
					Data string `json:"data"`
				} `json:"audio"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(result.Choices) == 0 || result.Choices[0].Message.Audio == nil || result.Choices[0].Message.Audio.Data == "" {
		return "", fmt.Errorf("no audio in response")
	}

	audioData, err := base64.StdEncoding.DecodeString(result.Choices[0].Message.Audio.Data)
	if err != nil {
		return "", fmt.Errorf("failed to decode audio: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "anyclaw_tts_*.wav")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer tmpFile.Close()
	if _, err := tmpFile.Write(audioData); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write audio: %w", err)
	}

	logger.InfoCF("voice", "Xiaomi MiMo TTS synthesis completed", map[string]any{"path": tmpFile.Name()})
	return tmpFile.Name(), nil
}

// DetectSynthesizer inspects cfg and returns a Synthesizer if a TTS-capable provider is configured.
// Priority: ANYCLAW_TTS_API_KEY (scheduler) > ANYCLAW_VOICE_API_KEY (backward-compat) >
//           XIAOMI_MIMO_API_KEY (Xiaomi TTS) > providers.xiaomi_mimo > model_list xiaomi_mimo/ >
//           providers.openai > model_list openai/.
func DetectSynthesizer(cfg *config.Config) Synthesizer {
	if cfg == nil {
		cfg = &config.Config{}
	}
	// New scheduler: dedicated TTS key (skipped for Groq endpoints).
	if key := os.Getenv("ANYCLAW_TTS_API_KEY"); key != "" {
		base := os.Getenv("ANYCLAW_TTS_API_BASE")
		if strings.Contains(strings.ToLower(base), "xiaomimimo.com") {
			return NewXiaomiMiMoSynthesizer(key, base, "")
		}
		return NewOpenAISynthesizer(key, base)
	}
	// Old scheduler: fall back to ANYCLAW_VOICE_API_KEY.
	if key := os.Getenv("ANYCLAW_VOICE_API_KEY"); key != "" {
		base := os.Getenv("ANYCLAW_VOICE_API_BASE")
		if strings.Contains(strings.ToLower(base), "xiaomimimo.com") {
			return NewXiaomiMiMoSynthesizer(key, base, "")
		}
		if !strings.Contains(strings.ToLower(base), "groq.com") {
			return NewOpenAISynthesizer(key, base)
		}
	}
	// Xiaomi MiMo: env or providers config
	if key := os.Getenv("XIAOMI_MIMO_API_KEY"); key != "" {
		base := os.Getenv("XIAOMI_MIMO_API_BASE")
		return NewXiaomiMiMoSynthesizer(key, base, "")
	}
	if key := cfg.Providers.XiaomiMiMo.APIKey; key != "" {
		return NewXiaomiMiMoSynthesizer(key, cfg.Providers.XiaomiMiMo.APIBase, cfg.Providers.XiaomiMiMo.TTSModel)
	}
	for _, mc := range cfg.ModelList {
		if strings.HasPrefix(mc.Model, "xiaomi_mimo/") && mc.APIKey != "" {
			model := strings.TrimPrefix(mc.Model, "xiaomi_mimo/")
			if model == "" {
				model = "mimo-v2-tts"
			}
			return NewXiaomiMiMoSynthesizer(mc.APIKey, mc.APIBase, model)
		}
	}
	// OpenAI
	if key := cfg.Providers.OpenAI.APIKey; key != "" {
		return NewOpenAISynthesizer(key, cfg.Providers.OpenAI.APIBase)
	}
	for _, mc := range cfg.ModelList {
		if strings.HasPrefix(mc.Model, "openai/") && mc.APIKey != "" {
			return NewOpenAISynthesizer(mc.APIKey, mc.APIBase)
		}
	}
	return nil
}
