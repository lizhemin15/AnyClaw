package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// DetectSynthesizer inspects cfg and returns a Synthesizer if OpenAI is configured.
func DetectSynthesizer(cfg *config.Config) Synthesizer {
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
