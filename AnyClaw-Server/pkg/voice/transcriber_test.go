package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/anyclaw/anyclaw-server/pkg/config"
)

// Ensure transcribers satisfy the Transcriber interface at compile time.
var _ Transcriber = (*GroqTranscriber)(nil)
var _ Transcriber = (*OpenAITranscriber)(nil)

func TestGroqTranscriberName(t *testing.T) {
	tr := NewGroqTranscriber("sk-test")
	if got := tr.Name(); got != "groq" {
		t.Errorf("Name() = %q, want %q", got, "groq")
	}
}

func TestOpenAITranscriberName(t *testing.T) {
	tr := NewOpenAITranscriber("sk-test", "")
	if got := tr.Name(); got != "openai" {
		t.Errorf("Name() = %q, want %q", got, "openai")
	}
}

func TestOpenAITranscriberDefaultBase(t *testing.T) {
	tr := NewOpenAITranscriber("sk-test", "")
	if tr.apiBase != "https://api.openai.com/v1" {
		t.Errorf("apiBase = %q, want %q", tr.apiBase, "https://api.openai.com/v1")
	}
}

func TestOpenAITranscriberCustomBase(t *testing.T) {
	tr := NewOpenAITranscriber("sk-test", "https://custom.example.com/v1")
	if tr.apiBase != "https://custom.example.com/v1" {
		t.Errorf("apiBase = %q, want %q", tr.apiBase, "https://custom.example.com/v1")
	}
}

func TestDetectTranscriber(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.Config
		wantNil  bool
		wantName string
	}{
		{
			name:    "no config",
			cfg:     &config.Config{},
			wantNil: true,
		},
		{
			name: "openai provider key",
			cfg: &config.Config{
				Providers: config.ProvidersConfig{
					OpenAI: config.OpenAIProviderConfig{ProviderConfig: config.ProviderConfig{APIKey: "sk-openai-direct"}},
				},
			},
			wantName: "openai",
		},
		{
			name: "openai via model list",
			cfg: &config.Config{
				ModelList: []config.ModelConfig{
					{Model: "openai/gpt-4o", APIKey: "sk-openai"},
				},
			},
			wantName: "openai",
		},
		{
			name: "openai provider takes priority over groq",
			cfg: &config.Config{
				Providers: config.ProvidersConfig{
					OpenAI: config.OpenAIProviderConfig{ProviderConfig: config.ProviderConfig{APIKey: "sk-openai-direct"}},
					Groq:   config.ProviderConfig{APIKey: "sk-groq-direct"},
				},
			},
			wantName: "openai",
		},
		{
			name: "groq provider key",
			cfg: &config.Config{
				Providers: config.ProvidersConfig{
					Groq: config.ProviderConfig{APIKey: "sk-groq-direct"},
				},
			},
			wantName: "groq",
		},
		{
			name: "groq via model list",
			cfg: &config.Config{
				ModelList: []config.ModelConfig{
					{Model: "groq/llama-3.3-70b", APIKey: "sk-groq-model"},
				},
			},
			wantName: "groq",
		},
		{
			name: "openai model list takes priority over groq model list",
			cfg: &config.Config{
				ModelList: []config.ModelConfig{
					{Model: "openai/gpt-4o", APIKey: "sk-openai"},
					{Model: "groq/llama-3.3-70b", APIKey: "sk-groq-model"},
				},
			},
			wantName: "openai",
		},
		{
			name: "groq model list entry without key is skipped",
			cfg: &config.Config{
				ModelList: []config.ModelConfig{
					{Model: "groq/llama-3.3-70b", APIKey: ""},
				},
			},
			wantNil: true,
		},
		{
			name: "openai model list entry without key is skipped",
			cfg: &config.Config{
				ModelList: []config.ModelConfig{
					{Model: "openai/gpt-4o", APIKey: ""},
				},
			},
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := DetectTranscriber(tc.cfg)
			if tc.wantNil {
				if tr != nil {
					t.Errorf("DetectTranscriber() = %v, want nil", tr)
				}
				return
			}
			if tr == nil {
				t.Fatal("DetectTranscriber() = nil, want non-nil")
			}
			if got := tr.Name(); got != tc.wantName {
				t.Errorf("Name() = %q, want %q", got, tc.wantName)
			}
		})
	}
}

func TestTranscribe(t *testing.T) {
	tmpDir := t.TempDir()
	audioPath := filepath.Join(tmpDir, "clip.ogg")
	if err := os.WriteFile(audioPath, []byte("fake-audio-data"), 0o644); err != nil {
		t.Fatalf("failed to write fake audio file: %v", err)
	}

	makeServer := func(t *testing.T, wantModel string) *httptest.Server {
		t.Helper()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/audio/transcriptions" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer sk-test" {
				t.Errorf("unexpected Authorization header: %s", r.Header.Get("Authorization"))
			}
			if err := r.ParseMultipartForm(1 << 20); err == nil {
				if got := r.FormValue("model"); got != wantModel {
					t.Errorf("model = %q, want %q", got, wantModel)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(TranscriptionResponse{
				Text:     "hello world",
				Language: "en",
				Duration: 1.5,
			})
		}))
	}

	t.Run("openai success", func(t *testing.T) {
		srv := makeServer(t, "whisper-1")
		defer srv.Close()

		tr := NewOpenAITranscriber("sk-test", srv.URL)
		resp, err := tr.Transcribe(context.Background(), audioPath)
		if err != nil {
			t.Fatalf("Transcribe() error: %v", err)
		}
		if resp.Text != "hello world" {
			t.Errorf("Text = %q, want %q", resp.Text, "hello world")
		}
	})

	t.Run("groq success", func(t *testing.T) {
		srv := makeServer(t, "whisper-large-v3")
		defer srv.Close()

		tr := NewGroqTranscriber("sk-test")
		tr.apiBase = srv.URL

		resp, err := tr.Transcribe(context.Background(), audioPath)
		if err != nil {
			t.Fatalf("Transcribe() error: %v", err)
		}
		if resp.Text != "hello world" {
			t.Errorf("Text = %q, want %q", resp.Text, "hello world")
		}
		if resp.Language != "en" {
			t.Errorf("Language = %q, want %q", resp.Language, "en")
		}
	})

	t.Run("api error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":"invalid_api_key"}`, http.StatusUnauthorized)
		}))
		defer srv.Close()

		tr := NewOpenAITranscriber("sk-bad", srv.URL)
		_, err := tr.Transcribe(context.Background(), audioPath)
		if err == nil {
			t.Fatal("expected error for non-200 response, got nil")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		tr := NewOpenAITranscriber("sk-test", "https://api.openai.com/v1")
		_, err := tr.Transcribe(context.Background(), filepath.Join(tmpDir, "nonexistent.ogg"))
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})
}
