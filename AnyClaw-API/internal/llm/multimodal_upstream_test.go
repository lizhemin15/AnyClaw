package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeMessagesMediaForUpstream(t *testing.T) {
	raw := []any{
		map[string]any{
			"role":    "user",
			"content": "[image: photo]",
			"media":   []any{"data:image/png;base64,abcd"},
		},
	}
	out := normalizeMessagesMediaForUpstream(raw)
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "image_url") || !strings.Contains(s, "data:image/png;base64,abcd") {
		t.Fatalf("expected image_url in JSON, got %s", s)
	}
	if strings.Contains(s, `"media"`) {
		t.Fatalf("media key should be stripped, got %s", s)
	}
}

func TestNormalizeMessagesMediaForUpstream_NoMediaPassthrough(t *testing.T) {
	raw := []any{
		map[string]any{"role": "user", "content": "hi"},
	}
	out := normalizeMessagesMediaForUpstream(raw)
	m := out[0].(map[string]any)
	if m["content"] != "hi" {
		t.Fatalf("content = %v", m["content"])
	}
}
