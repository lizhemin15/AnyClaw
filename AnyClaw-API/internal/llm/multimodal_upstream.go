package llm

import (
	"strings"
)

// normalizeMessagesMediaForUpstream converts AnyClaw pet-style messages (optional
// "media": []string of data URLs) into OpenAI-compatible multipart content before
// forwarding to /v1/chat/completions. Upstream providers ignore unknown "media".
func normalizeMessagesMediaForUpstream(msgs []any) []any {
	out := make([]any, len(msgs))
	for i, raw := range msgs {
		m, ok := raw.(map[string]any)
		if !ok {
			out[i] = raw
			continue
		}
		refs := extractStringSlice(m["media"])
		if len(refs) == 0 {
			m2 := shallowCopyMap(m)
			delete(m2, "media")
			out[i] = m2
			continue
		}
		parts := buildOpenAIContentParts(m["content"], refs)
		m2 := shallowCopyMap(m)
		delete(m2, "media")
		m2["content"] = parts
		out[i] = m2
	}
	return out
}

func shallowCopyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func extractStringSlice(v any) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func buildOpenAIContentParts(content any, media []string) []map[string]any {
	parts := make([]map[string]any, 0, 1+len(media))

	switch c := content.(type) {
	case string:
		if t := strings.TrimSpace(c); t != "" {
			parts = append(parts, map[string]any{"type": "text", "text": t})
		}
	case []any:
		for _, p := range c {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			parts = append(parts, pm)
		}
	}

	for _, u := range media {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		switch {
		case strings.HasPrefix(u, "data:image/"):
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": u,
				},
			})
		case strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://"):
			parts = append(parts, map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": u,
				},
			})
		}
	}

	if len(parts) == 0 {
		return []map[string]any{{"type": "text", "text": ""}}
	}
	return parts
}
