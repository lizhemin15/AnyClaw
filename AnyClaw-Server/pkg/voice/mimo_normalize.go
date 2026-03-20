package voice

import "strings"

// NormalizeXiaomiMisformedStyleTag fixes a common mistake: `<Happy>台词` is read aloud as the word
// "Happy". MiMo requires `<style>Happy</style>台词` or the speak tool's style= parameter.
func NormalizeXiaomiMisformedStyleTag(text string) string {
	s := strings.TrimSpace(text)
	if s == "" {
		return text
	}
	lo := strings.ToLower(s)
	if strings.HasPrefix(lo, "<style>") {
		return text
	}
	if s[0] != '<' {
		return text
	}
	end := strings.Index(s[1:], ">")
	if end < 0 {
		return text
	}
	end++ // closing '>' index in s
	inner := strings.TrimSpace(s[1:end])
	if inner == "" || strings.Contains(inner, "/") || strings.Contains(inner, "<") {
		return text
	}
	if strings.EqualFold(inner, "style") {
		return text
	}
	// avoid rewriting huge blobs mistaken for tags
	if len(inner) > 64 {
		return text
	}
	rest := strings.TrimSpace(s[end+1:])
	var b strings.Builder
	b.WriteString("<style>")
	b.WriteString(inner)
	b.WriteString("</style>")
	if rest != "" {
		b.WriteString(rest)
	}
	return b.String()
}
