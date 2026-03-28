package scheduler

import (
	"regexp"
	"strings"
)

const (
	defaultAgentIDForNormalize = "main"
	maxAgentIDLength           = 64
)

var (
	validIDRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
	invalidCharsRe = regexp.MustCompile(`[^a-z0-9_-]+`)
	leadingDashRe  = regexp.MustCompile(`^-+`)
	trailingDashRe = regexp.MustCompile(`-+$`)
)

// NormalizeAgentID 与 anyclaw-server/pkg/routing.NormalizeAgentID 一致，保证 API 从 config.json 解析的 slug 与容器内注册、同步的 id 对齐。
func NormalizeAgentID(id string) string {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return defaultAgentIDForNormalize
	}
	lower := strings.ToLower(trimmed)
	if validIDRe.MatchString(lower) {
		return lower
	}
	result := invalidCharsRe.ReplaceAllString(lower, "-")
	result = leadingDashRe.ReplaceAllString(result, "")
	result = trailingDashRe.ReplaceAllString(result, "")
	if len(result) > maxAgentIDLength {
		result = result[:maxAgentIDLength]
	}
	if result == "" {
		return defaultAgentIDForNormalize
	}
	return result
}
