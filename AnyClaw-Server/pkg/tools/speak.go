package tools

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/voice"
)

// SpeakTool allows the LLM to synthesize text to speech and send it as an audio message.
type SpeakTool struct {
	synthesizer    voice.Synthesizer
	mediaStore     media.MediaStore
	workspace      string
	defaultChannel string
	defaultChatID  string
}

func NewSpeakTool(synthesizer voice.Synthesizer, store media.MediaStore, workspace string) *SpeakTool {
	return &SpeakTool{
		synthesizer: synthesizer,
		mediaStore:  store,
		workspace:   workspace,
	}
}

func (t *SpeakTool) Name() string { return "speak" }

func (t *SpeakTool) Description() string {
	return "Synthesize text to speech using the same TTS backend as voice messages (NOT curl to ANYCLAW_VOICE OpenAI-compatible /audio/speech). Default send=true: push audio to user. For BGM mix / radio: send=false writes stem under workspace/.tts_staging/ for ffmpeg, then send_file the mix. When TTS is Xiaomi MiMo, use style= or MiMo tags per the voice skill."
}

func (t *SpeakTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to synthesize into speech. For Xiaomi MiMo, may include <style>…</style>, [cough], (laugh), long sigh, etc.",
			},
			"style": map[string]any{
				"type":        "string",
				"description": "Xiaomi MiMo only: overall style prepended as <style>…</style> (e.g. Happy, Whisper, 唱歌; multiple words allowed). Ignored for OpenAI. Omit if text already starts with <style>.",
			},
			"voice": map[string]any{
				"type":        "string",
				"description": "Voice ID. OpenAI: alloy, echo, fable, onyx, nova, shimmer. Xiaomi MiMo: mimo_default, default_zh, default_en. See voice skill for provider-specific options. Default depends on TTS provider.",
			},
			"send": map[string]any{
				"type":        "boolean",
				"description": "If true (default), send synthesized audio to the user. If false, only write stem to workspace/.tts_staging/ for ffmpeg+BGM; same engine as send=true—do not use curl /audio/speech.",
			},
		},
		"required": []string{"text"},
	}
}

func (t *SpeakTool) SetContext(channel, chatID string) {
	t.defaultChannel = channel
	t.defaultChatID = chatID
}

func (t *SpeakTool) SetSynthesizer(s voice.Synthesizer) {
	t.synthesizer = s
}

func (t *SpeakTool) SetMediaStore(store media.MediaStore) {
	t.mediaStore = store
}

func (t *SpeakTool) SetWorkspace(dir string) {
	t.workspace = dir
}

func parseSendToUserArg(args map[string]any) bool {
	switch v := args["send"].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	default:
		return true
	}
}

func copySpeakFile(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func (t *SpeakTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	text, _ := args["text"].(string)
	if text == "" {
		return ErrorResult("text is required")
	}
	voiceID, _ := args["voice"].(string)
	style, _ := args["style"].(string)
	if style != "" && t.synthesizer != nil && t.synthesizer.Name() == "xiaomi_mimo" {
		trimmed := strings.TrimSpace(text)
		if !strings.HasPrefix(strings.ToLower(trimmed), "<style>") {
			text = fmt.Sprintf("<style>%s</style>%s", strings.TrimSpace(style), text)
		}
	}
	sendToUser := parseSendToUserArg(args)

	channel := ToolChannel(ctx)
	if channel == "" {
		channel = t.defaultChannel
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = t.defaultChatID
	}
	if sendToUser && (channel == "" || chatID == "") {
		return ErrorResult("no target channel/chat available")
	}

	if t.synthesizer == nil {
		return ErrorResult("TTS not configured (no API key set for OpenAI or Xiaomi MiMo)")
	}
	if sendToUser && t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}

	path, err := t.synthesizer.Synthesize(ctx, text, voiceID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("TTS synthesis failed: %v", err)).WithError(err)
	}

	if !sendToUser {
		if strings.TrimSpace(t.workspace) == "" {
			return ErrorResult("send=false requires agent workspace; cannot write TTS stem")
		}
		staging := filepath.Join(t.workspace, ".tts_staging")
		if err := os.MkdirAll(staging, 0o755); err != nil {
			return ErrorResult(fmt.Sprintf("mkdir .tts_staging: %v", err)).WithError(err)
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == "" {
			ext = ".wav"
		}
		dest := filepath.Join(staging, fmt.Sprintf("stem_%d%s", time.Now().UnixNano(), ext))
		if err := copySpeakFile(dest, path); err != nil {
			return ErrorResult(fmt.Sprintf("failed to copy TTS stem: %v", err)).WithError(err)
		}
		_ = os.Remove(path)
		rel, err := filepath.Rel(t.workspace, dest)
		if err != nil {
			rel = dest
		}
		rel = filepath.ToSlash(rel)
		msg := fmt.Sprintf("TTS stem saved (same backend as speak to user). Relative path from workspace: %s\nUse with shell/ffmpeg under workspace cwd, then send_file the mixed audio. Do not re-synthesize via curl + ANYCLAW_VOICE /audio/speech.", rel)
		return SilentResult(msg)
	}

	filename := "speech.mp3"
	contentType := "audio/mpeg"
	if strings.HasSuffix(strings.ToLower(path), ".wav") {
		filename = "speech.wav"
		contentType = "audio/wav"
	}
	scope := fmt.Sprintf("tool:speak:%s:%s", channel, chatID)
	ref, err := t.mediaStore.Store(path, media.MediaMeta{
		Filename:    filename,
		ContentType: contentType,
		Source:      "tool:speak",
	}, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to store audio: %v", err)).WithError(err)
	}

	return MediaResult("Audio message sent to user", []string{ref})
}
