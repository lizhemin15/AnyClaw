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
		case int:
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
	return "Fetch roster: agents (agent_slug, display_name), peer_instances (linked other instances: instance_id, name), limits. Cross-instance contactability is determined only by peer_instances (not by topology edges). Use collab_find_peer_instance / collab_send_instance_message; same-instance mail uses collab_resolve_peer + internal_mail_send."
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
	return "Returns same-instance agent edges [[slug_lo,slug_hi],...] (for internal_mail neighbor checks only), plus peer_instances (account instance-to-instance links for cross-instance messaging), versions, limits. IMPORTANT: empty edges does NOT mean no cross-instance peers—judge cross-instance reachability only by peer_instances. internal_mail_send uses edges; collab_send_instance_message uses peer_instances."
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
	case int:
		if x < 0 {
			return defaultVal
		}
		return x
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

// CollabFindPeerInstanceTool 在编排拓扑的 peer_instances 中按名称查找对方实例（用于「联系某某」）。
type CollabFindPeerInstanceTool struct {
	client *CollabAPIClient
}

func NewCollabFindPeerInstanceTool(client *CollabAPIClient) *CollabFindPeerInstanceTool {
	return &CollabFindPeerInstanceTool{client: client}
}

func (t *CollabFindPeerInstanceTool) Name() string {
	return "collab_find_peer_instance"
}

func (t *CollabFindPeerInstanceTool) Description() string {
	return "Find a linked peer instance by display name among peer_instances from the account orchestration topology. Returns ordered matches (exact name first, then substring). Use before collab_send_instance_message when the user gives a colleague name instead of instance id."
}

func (t *CollabFindPeerInstanceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Colleague / instance display name (as shown in the roster)",
			},
		},
		"required": []string{"name"},
	}
}

func (t *CollabFindPeerInstanceTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	name, _ := args["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrorResult("name is required")
	}
	roster, err := t.client.GetRoster(ctx)
	if err != nil {
		return ErrorResult(err.Error())
	}
	nl := strings.ToLower(name)
	var exact, partial []map[string]any
	for _, p := range roster.PeerInstances {
		pn := strings.TrimSpace(p.Name)
		if pn == "" {
			continue
		}
		pl := strings.ToLower(pn)
		row := map[string]any{"instance_id": p.InstanceID, "name": p.Name}
		if pl == nl {
			row["match"] = "exact"
			exact = append(exact, row)
		} else if nl != "" && strings.Contains(pl, nl) {
			row["match"] = "partial"
			partial = append(partial, row)
		}
	}
	matches := append(exact, partial...)
	out := map[string]any{
		"matches":         matches,
		"peer_instances":  roster.PeerInstances,
		"not_found":       len(matches) == 0,
		"ambiguous":       len(matches) > 1,
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ErrorResult(err.Error())
	}
	return &ToolResult{ForLLM: string(b)}
}

// CollabSendInstanceMessageTool 跨实例发消息（须已在账号编排拓扑中连线）。
type CollabSendInstanceMessageTool struct {
	client *CollabAPIClient
}

func NewCollabSendInstanceMessageTool(client *CollabAPIClient) *CollabSendInstanceMessageTool {
	return &CollabSendInstanceMessageTool{client: client}
}

func (t *CollabSendInstanceMessageTool) Name() string {
	return "collab_send_instance_message"
}

func (t *CollabSendInstanceMessageTool) Description() string {
	return "Send a message to another instance (another lobster) linked in peer_instances (account instance-to-instance topology). API enforces that link—not the same-instance edges array. Body size cap: collab_get_roster.limits (max_instance_message_body_kb). Resolve to_instance_id via collab_get_roster or collab_find_peer_instance."
}

func (t *CollabSendInstanceMessageTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to_instance_id": map[string]any{
				"type":        "number",
				"description": "Target instance id (from peer_instances)",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Message body to deliver to the peer instance",
			},
		},
		"required": []string{"to_instance_id", "content"},
	}
}

func (t *CollabSendInstanceMessageTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	var toID int64
	switch v := args["to_instance_id"].(type) {
	case float64:
		toID = int64(v)
	case int:
		toID = int64(v)
	case int64:
		toID = v
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return ErrorResult("invalid to_instance_id")
		}
		toID = n
	default:
		return ErrorResult("to_instance_id is required")
	}
	content, _ := args["content"].(string)
	content = strings.TrimSpace(content)
	if toID < 1 || content == "" {
		return ErrorResult("to_instance_id and non-empty content are required")
	}
	out, err := t.client.PostInstanceMessage(ctx, toID, content)
	if err != nil {
		return ErrorResult(err.Error())
	}
	b, _ := json.Marshal(out)
	return &ToolResult{ForLLM: string(b)}
}

// CollabListInstanceMessagesTool 分页查询跨实例往来消息。
type CollabListInstanceMessagesTool struct {
	client *CollabAPIClient
}

func NewCollabListInstanceMessagesTool(client *CollabAPIClient) *CollabListInstanceMessagesTool {
	return &CollabListInstanceMessagesTool{client: client}
}

func (t *CollabListInstanceMessagesTool) Name() string {
	return "collab_list_instance_messages"
}

func (t *CollabListInstanceMessagesTool) Description() string {
	return "List cross-instance messages for this instance (paginated, newest-first semantics from API). Optional peer instance id filter. Response includes messages, total, limits. Call periodically when peer_instances exist (e.g. after idle time or if push may have been missed) to avoid missing inbound mail."
}

func (t *CollabListInstanceMessagesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"peer": map[string]any{
				"type":        "number",
				"description": "Optional: only messages with this peer instance id",
			},
			"limit": map[string]any{
				"type":        "number",
				"description": "Page size; omit or 0 for API default",
			},
			"offset": map[string]any{
				"type":        "number",
				"description": "Offset (default 0)",
			},
		},
		"required": []string{},
	}
}

func (t *CollabListInstanceMessagesTool) Execute(ctx context.Context, args map[string]any) *ToolResult {
	if t.client == nil {
		return ErrorResult("collab API not configured")
	}
	limit := collabArgNonNegInt(args, "limit", 0)
	offset := collabArgNonNegInt(args, "offset", 0)
	var peer *int64
	if v, ok := args["peer"]; ok && v != nil {
		switch x := v.(type) {
		case float64:
			n := int64(x)
			if n > 0 {
				peer = &n
			}
		case int:
			n := int64(x)
			if n > 0 {
				peer = &n
			}
		case int64:
			if x > 0 {
				peer = &x
			}
		case json.Number:
			n, err := x.Int64()
			if err == nil && n > 0 {
				peer = &n
			}
		}
	}
	out, err := t.client.GetInstanceMessageList(ctx, limit, offset, peer)
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
