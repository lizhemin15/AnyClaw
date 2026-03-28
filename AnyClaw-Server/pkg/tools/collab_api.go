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

func collabJSONInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case int:
		return int64(x), true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	default:
		return 0, false
	}
}

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

// CollabPeerInstance 账号编排拓扑中与当前实例直连的其它实例（跨实例协作）
type CollabPeerInstance struct {
	InstanceID int64  `json:"instance_id"`
	Name       string `json:"name"`
}

// CollabRosterResponse GET .../collab/bridge/roster
type CollabRosterResponse struct {
	Agents                  []CollabRosterAgent  `json:"agents"`
	PeerInstances           []CollabPeerInstance `json:"peer_instances"`
	InstanceTopologyVersion int64                `json:"instance_topology_version"`
	Limits                  *CollabLimits        `json:"limits,omitempty"`
}

// CollabTopologyResponse GET .../collab/bridge/topology
type CollabTopologyResponse struct {
	Edges                   [][2]string          `json:"edges"`
	Version                 int64                `json:"version"`
	PeerInstances           []CollabPeerInstance `json:"peer_instances"`
	InstanceTopologyVersion int64                `json:"instance_topology_version"`
	Limits                  *CollabLimits        `json:"limits,omitempty"`
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

// SyncRosterSlugs 将本容器 agents.list 的 id 合并进 API 协作表（仅追加新 slug）。
func (c *CollabAPIClient) SyncRosterSlugs(ctx context.Context, slugs []string) (added int, err error) {
	payload := map[string]any{"slugs": slugs}
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/roster/sync"
	req, err := c.req(ctx, http.MethodPost, rel, bytes.NewReader(raw))
	if err != nil {
		return 0, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("api %s: %s", resp.Status, string(bytes.TrimSpace(b)))
	}
	var out struct {
		Added int `json:"added"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, fmt.Errorf("decode sync roster: %w", err)
	}
	return out.Added, nil
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

// GetInstanceMessageList 分页拉取跨实例消息（容器 token；与网页端 GET collab/instance-mail 同形）。
func (c *CollabAPIClient) GetInstanceMessageList(ctx context.Context, limit, offset int, peer *int64) (map[string]any, error) {
	q := url.Values{}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		q.Set("offset", strconv.Itoa(offset))
	}
	if peer != nil && *peer > 0 {
		q.Set("peer", strconv.FormatInt(*peer, 10))
	}
	relPath := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/instance-mail"
	if enc := q.Encode(); enc != "" {
		relPath += "?" + enc
	}
	b, err := c.getOKBody(ctx, relPath)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("decode instance messages: %w", err)
	}
	return out, nil
}

// GetInstanceMessageByID returns one row from the instance-mail list API by message id.
// When peerID > 0, the list is filtered to that peer first (smaller result); falls back to unfiltered list.
func (c *CollabAPIClient) GetInstanceMessageByID(ctx context.Context, msgID int64, peerID int64) (map[string]any, error) {
	if msgID < 1 {
		return nil, fmt.Errorf("invalid message id")
	}
	try := func(peer *int64) (map[string]any, bool, error) {
		out, err := c.GetInstanceMessageList(ctx, 500, 0, peer)
		if err != nil {
			return nil, false, err
		}
		raw, ok := out["messages"].([]any)
		if !ok {
			return nil, false, fmt.Errorf("instance messages: missing messages array")
		}
		for _, m := range raw {
			row, ok := m.(map[string]any)
			if !ok {
				continue
			}
			id, ok := collabJSONInt64(row["id"])
			if ok && id == msgID {
				return row, true, nil
			}
		}
		return nil, false, nil
	}
	var peer *int64
	if peerID > 0 {
		p := peerID
		peer = &p
	}
	if row, found, err := try(peer); err != nil {
		return nil, err
	} else if found {
		return row, nil
	}
	if peer != nil {
		if row, found, err := try(nil); err != nil {
			return nil, err
		} else if found {
			return row, nil
		}
	}
	return nil, fmt.Errorf("instance message id %d not found", msgID)
}

// PostInstanceMessage 向编排拓扑中已连线的另一实例发送跨实例消息（容器 token）。
func (c *CollabAPIClient) PostInstanceMessage(ctx context.Context, toInstanceID int64, content string) (map[string]any, error) {
	raw, err := json.Marshal(map[string]any{
		"to_instance_id": toInstanceID,
		"content":        content,
	})
	if err != nil {
		return nil, err
	}
	rel := "/instances/" + url.PathEscape(c.InstanceID) + "/collab/bridge/instance-mail"
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
