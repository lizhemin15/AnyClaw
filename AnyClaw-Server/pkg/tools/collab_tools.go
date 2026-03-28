package tools

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// InternalMailSendTool 通过 API 发内部邮件（仅拓扑邻居可送达，由 API 校验）
type InternalMailSendTool struct {
	client *CollabAPIClient
}

func NewInternalMailSendTool(client *CollabAPIClient) *InternalMailSendTool {
	return &InternalMailSendTool{client: client}
}

func (t *InternalMailSendTool) Name() string {
	return "internal_mail_send"
}

func (t *InternalMailSendTool) Description() string {
	return "Send an internal mail to another agent. Only direct neighbors in the configured topology can receive. Use collab_resolve_peer first if you only have a display name. Size limits (thread_id, subject, body in KB) are in collab_get_roster.limits."
}

func (t *InternalMailSendTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_slug": map[string]any{
				"type":        "string",
				"description": "Your agent id (slug) as sender",
			},
			"to_slug": map[string]any{
				"type":        "string",
				"description": "Recipient agent id (slug); must be a topology neighbor",
			},
			"thread_id": map[string]any{
				"type":        "string",
				"description": "Conversation thread id (reuse for replies in the same thread)",
			},
			"subject": map[string]any{
				"type":        "string",
				"description": "Short subject line",
			},
			"body": map[string]any{
				"type":        "string",
				"description": "Mail body",
			},
			"in_reply_to": map[string]any{
				"type":        "number",
				"description": "Optional: previous internal mail id when replying on the same thread",
			},
		},
		"required": []string{"from_slug", "to_slug", "thread_id", "body"},
	}
}

func (t *InternalMailSendTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("internal mail API not configured")
	}
	from, _ := args["from_slug"].(string)
	to, _ := args["to_slug"].(string)
	thread, _ := args["thread_id"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	var inReply *int64
	if v, ok := args["in_reply_to"]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			n := int64(x)
			inReply = &n
		case int64:
			inReply = &x
		case json.Number:
			n, err := x.Int64()
			if err == nil {
				inReply = &n
			}
		}
	}
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	thread = strings.TrimSpace(thread)
	if from == "" || to == "" || thread == "" || strings.TrimSpace(body) == "" {
		return ErrorResult("from_slug, to_slug, thread_id and body are required")
	}
	out, err := t.client.PostInternalMail(ctx, from, to, subject, body, thread, inReply)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, _ := json.Marshal(out)
	return &ToolResult{ForLLM: string(b)}
}

// CollabResolvePeerTool 将展示名解析为 agent_slug（本实例 roster，服务端规则）
type CollabResolvePeerTool struct {
	client *CollabAPIClient
}

func NewCollabResolvePeerTool(client *CollabAPIClient) *CollabResolvePeerTool {
	return &CollabResolvePeerTool{client: client}
}

func (t *CollabResolvePeerTool) Name() string {
	return "collab_resolve_peer"
}

func (t *CollabResolvePeerTool) Description() string {
	return "Resolve a colleague's display name to agent_slug for this instance. Returns ambiguous list if multiple matches."
}

func (t *CollabResolvePeerTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Display name or prefix as configured in manager",
			},
		},
		"required": []string{"name"},
	}
}

func (t *CollabResolvePeerTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrorResult("name is required")
	}
	out, err := t.client.PostResolve(ctx, name)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, _ := json.Marshal(out)
	return &ToolResult{ForLLM: string(b)}
}

// CollabGetRosterTool 拉取本实例协作员工列表与上限（API 与 manager 配置一致）。
type CollabGetRosterTool struct {
	client *CollabAPIClient
}

func NewCollabGetRosterTool(client *CollabAPIClient) *CollabGetRosterTool {
	return &CollabGetRosterTool{client: client}
}

func (t *CollabGetRosterTool) Name() string {
	return "collab_get_roster"
}

func (t *CollabGetRosterTool) Description() string {
	return "Fetch this instance's collaboration roster: same-instance agents (agent_slug, display_name), peer_instances (other instances connected in account orchestration topology, with instance_id and name), instance_topology_version, and server limits (max_agents, max_edges, mail caps, etc.). Use before internal_mail_send when you need current colleagues or limits."
}

func (t *CollabGetRosterTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *CollabGetRosterTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	_ = args
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	out, err := t.client.GetRoster(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return &ToolResult{ForLLM: string(b)}
}

// CollabGetTopologyTool 拉取无向邻居边与拓扑版本、上限。
type CollabGetTopologyTool struct {
	client *CollabAPIClient
}

func NewCollabGetTopologyTool(client *CollabAPIClient) *CollabGetTopologyTool {
	return &CollabGetTopologyTool{client: client}
}

func (t *CollabGetTopologyTool) Name() string {
	return "collab_get_topology"
}

func (t *CollabGetTopologyTool) Description() string {
	return "Fetch same-instance agent neighbor edges [[slug_lo,slug_hi],...], agent topology version, peer_instances (cross-instance orchestration links: instance_id, name), instance_topology_version, and limits. internal_mail_send neighbors are only along edges; peer_instances are linked instances for cross-instance collaboration awareness."
}

func (t *CollabGetTopologyTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *CollabGetTopologyTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	_ = args
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	out, err := t.client.GetTopology(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return &ToolResult{ForLLM: string(b)}
}

// CollabListInternalMailsTool 分页查询内部邮件列表（容器 token）。
type CollabListInternalMailsTool struct {
	client *CollabAPIClient
}

func NewCollabListInternalMailsTool(client *CollabAPIClient) *CollabListInternalMailsTool {
	return &CollabListInternalMailsTool{client: client}
}

func (t *CollabListInternalMailsTool) Name() string {
	return "collab_list_internal_mails"
}

func (t *CollabListInternalMailsTool) Description() string {
	return "List internal mails for this instance (paginated). Optional thread_id filter. Response includes total, mails (with body), and limits. Use after collab_get_roster for caps."
}

func (t *CollabListInternalMailsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"thread_id": map[string]any{
				"type":        "string",
				"description": "Optional: only mails in this thread_id",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Page size; omit or 0 for API default (100)",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "Offset for pagination (default 0)",
			},
		},
		"required": []string{},
	}
}

func collabArgNonNegInt(args map[string]any, key string, defaultVal int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return defaultVal
	}
	switch x := v.(type) {
	case float64:
		n := int(x)
		if n < 0 {
			return defaultVal
		}
		return n
	case int64:
		if int(x) < 0 {
			return defaultVal
		}
		return int(x)
	case json.Number:
		i64, err := x.Int64()
		if err != nil || i64 < 0 {
			return defaultVal
		}
		return int(i64)
	default:
		return defaultVal
	}
}

func (t *CollabListInternalMailsTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	thread, _ := args["thread_id"].(string)
	limit := collabArgNonNegInt(args, "limit", 0)
	offset := collabArgNonNegInt(args, "offset", 0)
	out, err := t.client.GetInternalMailList(ctx, thread, limit, offset)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return &ToolResult{ForLLM: string(b)}
}

// CollabMailIDFromNotify 从 API 下行 payload 解析 mail id
func CollabMailIDFromNotify(payload map[string]any) (int64, bool) {
	v, ok := payload["id"]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return int64(x), true
	case int64:
		return x, true
	case json.Number:
		n, err := x.Int64()
		return n, err == nil
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}
