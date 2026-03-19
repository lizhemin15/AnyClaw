package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/config"
	"github.com/anyclaw/anyclaw-server/pkg/logger"
	"github.com/anyclaw/anyclaw-server/pkg/utils"
)

type Transcriber interface {
	Name() string
	Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error)
}

type TranscriptionResponse struct {
	Text     string  `json:"text"`
	Language string  `json:"language,omitempty"`
	Duration float64 `json:"duration,omitempty"`
}

// whisperTranscribe is the shared HTTP implementation for OpenAI-compatible Whisper APIs.
func whisperTranscribe(ctx context.Context, apiKey, apiBase, model string, client *http.Client, audioFilePath string) (*TranscriptionResponse, error) {
	logger.InfoCF("voice", "Starting transcription", map[string]any{"audio_file": audioFilePath, "model": model})

	audioFile, err := os.Open(audioFilePath)
	if err != nil {
		logger.ErrorCF("voice", "Failed to open audio file", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to open audio file: %w", err)
	}
	defer audioFile.Close()

	fileInfo, err := audioFile.Stat()
	if err != nil {
		logger.ErrorCF("voice", "Failed to get file info", map[string]any{"path": audioFilePath, "error": err})
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	logger.DebugCF("voice", "Audio file details", map[string]any{
		"size_bytes": fileInfo.Size(),
		"file_name":  filepath.Base(audioFilePath),
	})

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	part, err := writer.CreateFormFile("file", filepath.Base(audioFilePath))
	if err != nil {
		logger.ErrorCF("voice", "Failed to create form file", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	copied, err := io.Copy(part, audioFile)
	if err != nil {
		logger.ErrorCF("voice", "Failed to copy file content", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to copy file content: %w", err)
	}

	logger.DebugCF("voice", "File copied to request", map[string]any{"bytes_copied": copied})

	if err = writer.WriteField("model", model); err != nil {
		logger.ErrorCF("voice", "Failed to write model field", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to write model field: %w", err)
	}

	if err = writer.WriteField("response_format", "json"); err != nil {
		logger.ErrorCF("voice", "Failed to write response_format field", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to write response_format field: %w", err)
	}

	if err = writer.Close(); err != nil {
		logger.ErrorCF("voice", "Failed to close multipart writer", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	reqURL := apiBase + "/audio/transcriptions"
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL, &requestBody)
	if err != nil {
		logger.ErrorCF("voice", "Failed to create request", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+apiKey)

	logger.DebugCF("voice", "Sending transcription request", map[string]any{
		"url":                reqURL,
		"request_size_bytes": requestBody.Len(),
		"file_size_bytes":    fileInfo.Size(),
	})

	resp, err := client.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send request", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read response", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF("voice", "API error", map[string]any{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.DebugCF("voice", "Received transcription response", map[string]any{
		"status_code":         resp.StatusCode,
		"response_size_bytes": len(body),
	})

	var result TranscriptionResponse
	if err := json.Unmarshal(body, &result); err != nil {
		logger.ErrorCF("voice", "Failed to unmarshal response", map[string]any{"error": err})
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	logger.InfoCF("voice", "Transcription completed successfully", map[string]any{
		"text_length":           len(result.Text),
		"language":              result.Language,
		"duration_seconds":      result.Duration,
		"transcription_preview": utils.Truncate(result.Text, 50),
	})

	return &result, nil
}

// OpenAITranscriber uses OpenAI Whisper (whisper-1) for transcription.
type OpenAITranscriber struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

func NewOpenAITranscriber(apiKey, apiBase string) *OpenAITranscriber {
	if apiBase == "" {
		apiBase = "https://api.chatanywhere.org/v1"
	}
	logger.DebugCF("voice", "Creating OpenAI transcriber", map[string]any{"has_api_key": apiKey != "", "api_base": apiBase})
	return &OpenAITranscriber{
		apiKey:  apiKey,
		apiBase: apiBase,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (t *OpenAITranscriber) Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error) {
	return whisperTranscribe(ctx, t.apiKey, t.apiBase, "whisper-1", t.httpClient, audioFilePath)
}

func (t *OpenAITranscriber) Name() string {
	return "openai"
}

// GroqTranscriber uses Groq Whisper (whisper-large-v3) for transcription.
type GroqTranscriber struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

func NewGroqTranscriber(apiKey string) *GroqTranscriber {
	logger.DebugCF("voice", "Creating Groq transcriber", map[string]any{"has_api_key": apiKey != ""})
	return &GroqTranscriber{
		apiKey:  apiKey,
		apiBase: "https://api.groq.com/openai/v1",
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (t *GroqTranscriber) Transcribe(ctx context.Context, audioFilePath string) (*TranscriptionResponse, error) {
	return whisperTranscribe(ctx, t.apiKey, t.apiBase, "whisper-large-v3", t.httpClient, audioFilePath)
}

func (t *GroqTranscriber) Name() string {
	return "groq"
}

// DetectTranscriber inspects cfg and returns the appropriate Transcriber, or
// nil if no supported transcription provider is configured.
// Priority: OpenAI > Groq.
func DetectTranscriber(cfg *config.Config) Transcriber {
	// OpenAI provider takes highest priority.
	if key := cfg.Providers.OpenAI.APIKey; key != "" {
		return NewOpenAITranscriber(key, cfg.Providers.OpenAI.APIBase)
	}
	// Check model list for openai/ protocol entries.
	for _, mc := range cfg.ModelList {
		if strings.HasPrefix(mc.Model, "openai/") && mc.APIKey != "" {
			return NewOpenAITranscriber(mc.APIKey, mc.APIBase)
		}
	}
	// Fall back to Groq provider config.
	if key := cfg.Providers.Groq.APIKey; key != "" {
		return NewGroqTranscriber(key)
	}
	// Fall back to any model-list entry that uses the groq/ protocol.
	for _, mc := range cfg.ModelList {
		if strings.HasPrefix(mc.Model, "groq/") && mc.APIKey != "" {
			return NewGroqTranscriber(mc.APIKey)
		}
	}
	return nil
}
