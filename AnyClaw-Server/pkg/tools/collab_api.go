package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CollabAPIClient 调用 AnyClaw-API 协作接口（容器 token）
type CollabAPIClient struct {
	BaseURL    string
	InstanceID string
	Token      string
	HTTP       *http.Client
}

func NewCollabAPIClient(baseURL, instanceID, token string) *CollabAPIClient {
	return &CollabAPIClient{
		BaseURL:    strings.TrimSuffix(strings.TrimSpace(baseURL), "/"),
		InstanceID: strings.TrimSpace(instanceID),
		Token:      strings.TrimSpace(token),
		HTTP:       &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *CollabAPIClient) req(ctx context.Context, method, relPath string, body io.Reader) (*http.Request, error) {
	if c.BaseURL == "" || c.InstanceID == "" || c.Token == "" {
		return nil, fmt.Errorf("collab api client not configured")
	}
	u := c.BaseURL + relPath
	if strings.Contains(relPath, "?") {
		u += "&token=" + url.QueryEscape(c.Token)
	} else {
		u += "?token=" + url.QueryEscape(c.Token)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// CollabLimits 与 AnyClaw-API collaborationLimitsPayload 一致（容器/管理端 GET 可选返回）。
type CollabLimits struct {
	MaxAgents                   int `json:"max_agents"`
	MaxEdges                    int `json:"max_edges"`
	MaxThreadIDRunes            int `json:"max_thread_id_runes"`
	MaxInternalMailSubjectRunes int `json:"max_internal_mail_subject_runes"`
	MaxInternalMailBodyKB       int `json:"max_internal_mail_body_kb"`
	MaxAgentSlugRunes           int `json:"max_agent_slug_runes"`
	MaxAgentDisplayNameRunes    int `json:"max_agent_display_name_runes"`
	MaxInternalMailListLimit    int `json:"max_internal_mail_list_limit"`
	MaxInternalMailListOffset   int `json:"max_internal_mail_list_offset"`
}

// CollabRosterAgent 对应 instance_agents 行 JSON。
type CollabRosterAgent struct {
	ID          int64  `json:"id"`
	InstanceID  int64  `json:"instance_id"`
	UserID      int64  `json:"user_id"`
	AgentSlug   string `json:"agent_slug"`
	DisplayName string `json:"display_name"`
}

// CollabRosterResponse GET .../collab/bridge/roster
type CollabRosterResponse struct {
	Agents []CollabRosterAgent `json:"agents"`
	Limits *CollabLimits       `json:"limits,omitempty"`
}

// CollabTopologyResponse GET .../collab/bridge/topology
type CollabTopologyResponse struct {
	Edges   [][2]string `json:"edges"`
	Version int64       `json:"version"`
	Limits  *CollabLimits `json:"limits,omitempty"`
}

func (c *CollabAPIClient) getOKBody(ctx context.Context, rel string) ([]byte, error) {
	req, err := c.req(ctx, http.MethodGet, rel, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api %s: %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	return b, nil
}

// GetRoster 拉取本实例协作员工与上限（容器 token）。
func (c *CollabAPIClient) GetRoster(ctx context.Context) (*CollabRosterResponse, error) {
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/roster"
	b, err := c.getOKBody(ctx, rel)
	if err != nil {
		return nil, err
	}
	var out CollabRosterResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode roster: %w", err)
	}
	return &out, nil
}

// GetTopology 拉取无向边、拓扑版本与上限（容器 token）。
func (c *CollabAPIClient) GetTopology(ctx context.Context) (*CollabTopologyResponse, error) {
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/topology"
	b, err := c.getOKBody(ctx, rel)
	if err != nil {
		return nil, err
	}
	var out CollabTopologyResponse
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode topology: %w", err)
	}
	return &out, nil
}

// GetInternalMailList 分页拉取内部邮件（容器 token；与 Manager GET collab/mails 同形：mails、total、limits）。
func (c *CollabAPIClient) GetInternalMailList(ctx context.Context, threadID string, limit, offset int) (map[string]any, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if t := strings.TrimSpace(threadID); t != "" {
		q.Set("thread_id", t)
	}
	relPath := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/mails"
	if enc := q.Encode(); enc != "" {
		relPath += "?" + enc
	}
	b, err := c.getOKBody(ctx, relPath)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode mail list: %w", err)
	}
	return out, nil
}

// PostInternalMail 发送内部邮件（邻居关系由 API 校验）
func (c *CollabAPIClient) PostInternalMail(ctx context.Context, fromSlug, toSlug, subject, body, threadID string, inReplyTo *int64) (map[string]any, error) {
	payload := map[string]any{
		"from_slug":  fromSlug,
		"to_slug":    toSlug,
		"subject":    subject,
		"body":       body,
		"thread_id":  threadID,
		"in_reply_to": nil,
	}
	if inReplyTo != nil {
		payload["in_reply_to"] = *inReplyTo
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/mail"
	req, err := c.req(ctx, http.MethodPost, rel, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api %s: %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// PostResolve 展示名解析为 agent_slug
func (c *CollabAPIClient) PostResolve(ctx context.Context, name string) (map[string]any, error) {
	raw, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return nil, err
	}
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/resolve"
	req, err := c.req(ctx, http.MethodPost, rel, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api %s: %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return out, nil
}

// GetInternalMail 拉取单封邮件 JSON（含 body）
func (c *CollabAPIClient) GetInternalMail(ctx context.Context, mailID int64) (map[string]any, error) {
	rel := fmt.Sprintf("/instances/%s/collab/bridge/mail/%d", url.PathEscape(c.InstanceID), mailID)
	b, err := c.getOKBody(ctx, rel)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return out, nil
}
