package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/anyclaw/anyclaw-server/pkg/media"
	"github.com/anyclaw/anyclaw-server/pkg/voice"
)

// SpeakTool allows the LLM to synthesize text to speech and send it as an audio message.
type SpeakTool struct {
	synthesizer    voice.Synthesizer
	mediaStore     media.MediaStore
	defaultChannel string
	defaultChatID  string
}

func NewSpeakTool(synthesizer voice.Synthesizer, store media.MediaStore) *SpeakTool {
	return &SpeakTool{
		synthesizer: synthesizer,
		mediaStore:  store,
	}
}

func (t *SpeakTool) Name() string { return "speak" }

func (t *SpeakTool) Description() string {
	return "Synthesize text to speech and send it as an audio message to the user. Use when the user requests a voice reply or when audio output is appropriate. When TTS is Xiaomi MiMo, prefer richer delivery: use optional style (e.g. Happy, Whisper, 唱歌) and/or embed MiMo tags in text (<style>…</style>, [cough], (sobbing), long sigh, etc.) per the voice skill."
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

	channel := ToolChannel(ctx)
	if channel == "" {
		channel = t.defaultChannel
	}
	chatID := ToolChatID(ctx)
	if chatID == "" {
		chatID = t.defaultChatID
	}
	if channel == "" || chatID == "" {
		return ErrorResult("no target channel/chat available")
	}

	if t.synthesizer == nil {
		return ErrorResult("TTS not configured (no API key set for OpenAI or Xiaomi MiMo)")
	}
	if t.mediaStore == nil {
		return ErrorResult("media store not configured")
	}

	path, err := t.synthesizer.Synthesize(ctx, text, voiceID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("TTS synthesis failed: %v", err)).WithError(err)
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
