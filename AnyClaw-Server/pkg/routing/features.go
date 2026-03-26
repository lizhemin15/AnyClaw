package routing

import (
	"strings"
	"unicode/utf8"

	"github.com/anyclaw/anyclaw-server/pkg/providers"
)

// lookbackWindow is the number of recent history entries scanned for tool calls.
// Six entries covers roughly one full tool-use round-trip (user ->assistant+tool_call ->tool_result ->assistant).
const lookbackWindow = 6

// Features holds the structural signals extracted from a message and its session context.
// Token and code-block signals avoid locale-specific keywords; attachment detection also
// recognizes fixed inbound channel markers (e.g. "[image: photo]") in addition to URLs
// and data URIs.
type Features struct {
	// TokenEstimate is a proxy for token count.
	// CJK runes count as 1 token each; non-CJK runes as 0.25 tokens each.
	// This avoids API calls while giving accurate estimates for all scripts.
	TokenEstimate int

	// CodeBlockCount is the number of fenced code blocks (``` pairs) in the message.
	// Coding tasks almost always require the heavy model.
	CodeBlockCount int

	// RecentToolCalls is the count of tool_call messages in the last lookbackWindow
	// history entries. A high density indicates an active agentic workflow.
	RecentToolCalls int

	// ConversationDepth is the total number of messages in the session history.
	// Deep sessions tend to carry implicit complexity built up over many turns.
	ConversationDepth int

	// HasAttachments is true when the message appears to contain media (images,
	// audio, video), including channel-normalized placeholders such as
	// "[image: photo]" when inbound pipelines attach binary parts. Multi-modal
	// inputs require vision-capable heavy models.
	HasAttachments bool
}

// ExtractFeatures computes the structural feature vector for a message.
// It is a pure function with no side effects and zero allocations beyond
// the returned struct.
func ExtractFeatures(msg string, history []providers.Message) Features {
	return Features{
		TokenEstimate:     estimateTokens(msg),
		CodeBlockCount:    countCodeBlocks(msg),
		RecentToolCalls:   countRecentToolCalls(history),
		ConversationDepth: len(history),
		HasAttachments:    hasAttachments(msg),
	}
}

// estimateTokens returns a token count proxy that handles both CJK and Latin text.
// CJK runes (U+2E80–U+9FFF, U+F900–U+FAFF, U+AC00–U+D7AF) map to roughly one
// token each, while non-CJK runes average ~0.25 tokens/rune (-> chars per token
// for English). Splitting the count this way avoids the 3x underestimation that a
// flat rune_count/3 would produce for Chinese, Japanese, and Korean text.
func estimateTokens(msg string) int {
	total := utf8.RuneCountInString(msg)
	if total == 0 {
		return 0
	}
	cjk := 0
	for _, r := range msg {
		if r >= 0x2E80 && r <= 0x9FFF || r >= 0xF900 && r <= 0xFAFF || r >= 0xAC00 && r <= 0xD7AF {
			cjk++
		}
	}
	return cjk + (total-cjk)/4
}

// countCodeBlocks counts the number of complete fenced code blocks.
// Each ``` delimiter increments a counter; pairs of delimiters form one block.
// An unclosed opening fence (odd count) is treated as zero complete blocks
// since it may just be an inline code span or a typo.
func countCodeBlocks(msg string) int {
	n := strings.Count(msg, "```")
	return n / 2
}

// countRecentToolCalls counts messages with tool calls in the last lookbackWindow
// entries of history. It examines the ToolCalls field rather than parsing
// the content string, so it is robust to any message format.
func countRecentToolCalls(history []providers.Message) int {
	start := len(history) - lookbackWindow
	if start < 0 {
		start = 0
	}

	count := 0
	for _, msg := range history[start:] {
		if len(msg.ToolCalls) > 0 {
			count += len(msg.ToolCalls)
		}
	}
	return count
}

// hasAttachments returns true when the message content contains embedded media.
// It checks for base64 data URIs (data:image/, data:audio/, data:video/) and
// common image/audio URL extensions. This is intentionally conservative -// false negatives (missing an attachment) just mean the routing falls back to
// the primary model anyway.
func hasAttachments(msg string) bool {
	lower := strings.ToLower(msg)

	// Base64 data URIs embedded directly in the message
	if strings.Contains(lower, "data:image/") ||
		strings.Contains(lower, "data:audio/") ||
		strings.Contains(lower, "data:video/") {
		return true
	}

	// Common image/audio extensions in URLs or file references
	mediaExts := []string{
		".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp",
		".mp3", ".wav", ".ogg", ".m4a", ".flac",
		".mp4", ".avi", ".mov", ".webm",
	}
	for _, ext := range mediaExts {
		if strings.Contains(lower, ext) {
			return true
		}
	}

	// Inbound channels (Feishu, Telegram, LINE, WeixinClaw, etc.) append fixed
	// English tags when media was downloaded for the LLM. Without this, short
	// captions route to light text-only models while the request still carries
	// image parts — a production footgun. These markers are server-controlled,
	// not end-user prose.
	if hasInboundMediaPlaceholder(msg) {
		return true
	}

	return false
}

// hasInboundMediaPlaceholder reports tags produced by AnyClaw channel adapters
// when associating binary media with a user turn.
func hasInboundMediaPlaceholder(msg string) bool {
	if msg == "" {
		return false
	}
	switch {
	case strings.Contains(msg, "[image:"):
		return true
	case strings.Contains(msg, "[audio]"):
		return true
	case strings.Contains(msg, "[video]"):
		return true
	case strings.Contains(msg, "[file]"):
		return true
	case strings.Contains(msg, "[attachment]"):
		return true
	default:
		return false
	}
}

// AnyMessageHasMedia returns true if any message carries Media refs or resolved
// data URLs (used after BuildMessages + resolveMediaRefs). This is a second line
// of defense when the visible user string does not include placeholders.
func AnyMessageHasMedia(msgs []providers.Message) bool {
	for i := range msgs {
		if len(msgs[i].Media) > 0 {
			return true
		}
	}
	return false
}
