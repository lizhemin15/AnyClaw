package tools

import (
	"context"
	"fmt"

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
	return "Synthesize text to speech and send it as an audio message to the user. Use when the user requests a voice reply or when audio output is appropriate."
}

func (t *SpeakTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "Text to synthesize into speech.",
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

	scope := fmt.Sprintf("tool:speak:%s:%s", channel, chatID)
	ref, err := t.mediaStore.Store(path, media.MediaMeta{
		Filename:    "speech.mp3",
		ContentType: "audio/mpeg",
		Source:      "tool:speak",
	}, scope)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to store audio: %v", err)).WithError(err)
	}

	return MediaResult("Audio message sent to user", []string{ref})
}
