package api_proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-server/pkg/providers/protocoltypes"
)

const (
	llmPath            = "/llm/v1/chat/completions"
	defaultReqTimeout  = 300 * time.Second // 5 分钟，应对慢速 LLM 或网络延迟
)

type (
	Message        = protocoltypes.Message
	ToolDefinition = protocoltypes.ToolDefinition
	LLMResponse   = protocoltypes.LLMResponse
	UsageInfo     = protocoltypes.UsageInfo
)

// Provider forwards LLM requests to AnyClaw-API. The API holds real keys;
// this provider only sends instance token for auth.
type Provider struct {
	apiURL     string
	instanceID string
	token      string
	httpClient *http.Client
}

// NewProvider creates an API proxy provider.
func NewProvider(apiURL, instanceID, token string) *Provider {
	base := strings.TrimSuffix(apiURL, "/")
	if !strings.HasPrefix(base, "http") {
		base = "https://" + base
	}
	return &Provider{
		apiURL:     base,
		instanceID: instanceID,
		token:      token,
		httpClient: &http.Client{Timeout: defaultReqTimeout},
	}
}

// UsesManagerProxy reports that LLM calls go through AnyClaw-Manager. The agent
// skips local light/heavy routing so logs and fallback attempts are not tied to
// pet-side model names that are ignored by default (see Chat).
func (p *Provider) UsesManagerProxy() bool { return true }

// Chat forwards the request to the API.
// By default the JSON body does not include "model": AnyClaw-Manager then uses
// GetEnabledModel() and FindChannelsForModel, so the pet needs no matching
// agents.defaults.model_name / model_list for LLM routing. Set ANYCLAW_API_PROXY_FORWARD_MODEL=1
// to forward the pet-resolved model name (stripped protocol prefix) for overrides.
func (p *Provider) Chat(
	ctx context.Context,
	messages []Message,
	tools []ToolDefinition,
	model string,
	options map[string]any,
) (*LLMResponse, error) {
	requestBody := map[string]any{
		"messages": serializeMessages(messages),
	}
	if forwardPetModel() {
		if m := modelIDForManager(model); m != "" {
			requestBody["model"] = m
		}
	}

	if len(tools) > 0 {
		requestBody["tools"] = tools
		requestBody["tool_choice"] = "auto"
	}

	if maxTokens, ok := asInt(options["max_tokens"]); ok {
		requestBody["max_tokens"] = maxTokens
	}
	if temperature, ok := asFloat(options["temperature"]); ok {
		requestBody["temperature"] = temperature
	}
	if cacheKey, ok := options["prompt_cache_key"].(string); ok && cacheKey != "" {
		requestBody["prompt_cache_key"] = cacheKey
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("api_proxy: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiURL+llmPath, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("api_proxy: create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("X-Instance-ID", p.instanceID)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api_proxy: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("api_proxy: status %d: %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	out, err := parseResponse(reader)
	if err != nil {
		return nil, fmt.Errorf("api_proxy: parse response: %w", err)
	}
	return out, nil
}

// GetDefaultModel returns empty; model comes from config.
func (p *Provider) GetDefaultModel() string {
	return ""
}

func forwardPetModel() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ANYCLAW_API_PROXY_FORWARD_MODEL")))
	return v == "1" || v == "true" || v == "yes"
}

// modelIDForManager strips a protocol prefix (e.g. openai/gpt-4o -> gpt-4o) so the
// manager matches channel model names as configured in the admin UI.
func modelIDForManager(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return ""
	}
	if _, rest, ok := strings.Cut(model, "/"); ok {
		return strings.TrimSpace(rest)
	}
	return model
}

func serializeMessages(messages []Message) []map[string]any {
	out := make([]map[string]any, 0, len(messages))
	for _, m := range messages {
		obj := map[string]any{"role": m.Role, "content": m.Content}
		if len(m.Media) > 0 {
			obj["media"] = m.Media
		}
		if m.ReasoningContent != "" {
			obj["reasoning_content"] = m.ReasoningContent
		}
		if len(m.ToolCalls) > 0 {
			obj["tool_calls"] = m.ToolCalls
		}
		if m.ToolCallID != "" {
			obj["tool_call_id"] = m.ToolCallID
		}
		out = append(out, obj)
	}
	return out
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	default:
		return 0, false
	}
}

func parseResponse(body io.Reader) (*LLMResponse, error) {
	var apiResp struct {
		Choices []struct {
			Message struct {
				Content          string                   `json:"content"`
				ReasoningContent string                   `json:"reasoning_content"`
				Reasoning        string                   `json:"reasoning"`
				ReasoningDetails []protocoltypes.ReasoningDetail `json:"reasoning_details"`
				ToolCalls        []protocoltypes.ToolCall `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *UsageInfo `json:"usage"`
	}
	if err := json.NewDecoder(body).Decode(&apiResp); err != nil {
		return nil, err
	}
	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	msg := apiResp.Choices[0].Message
	return &LLMResponse{
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		Reasoning:        msg.Reasoning,
		ReasoningDetails: msg.ReasoningDetails,
		ToolCalls:        msg.ToolCalls,
		FinishReason:     apiResp.Choices[0].FinishReason,
		Usage:            apiResp.Usage,
	}, nil
}
