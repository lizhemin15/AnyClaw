package collab

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/anyclaw/anyclaw-api/internal/ws"
	"github.com/go-chi/chi/v5"
)

// Handler 多员工协作 API（拓扑、 roster、内部邮件、指人解析）
type Handler struct {
	db  *db.DB
	hub *ws.Hub
}

func New(database *db.DB, hub *ws.Hub) *Handler {
	return &Handler{db: database, hub: hub}
}

func collaborationLimitsPayload() map[string]int {
	return map[string]int{
		"max_agents":                       db.MaxCollaborationAgentsPerInstance,
		"max_edges":                        db.MaxCollaborationEdgesPerInstance,
		"max_thread_id_runes":              db.MaxInternalMailThreadRunes,
		"max_internal_mail_subject_runes":  db.MaxInternalMailSubjectRunes,
		"max_internal_mail_body_kb":        db.MaxInternalMailBodyBytes / 1024,
		"max_agent_slug_runes":             db.MaxInstanceAgentSlugRunes,
		"max_agent_display_name_runes":     db.MaxInstanceAgentDisplayRunes,
		"max_internal_mail_list_limit":     db.MaxInternalMailListLimit,
		"max_internal_mail_list_offset":    db.MaxInternalMailListOffset,
	}
}

func instanceToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return r.URL.Query().Get("token")
}

func (h *Handler) authInstance(w http.ResponseWriter, r *http.Request) (*db.Instance, int64, bool) {
	idStr := chi.URLParam(r, "id")
	token := instanceToken(r)
	if idStr == "" || token == "" {
		http.Error(w, `{"error":"instance id and token required"}`, http.StatusBadRequest)
		return nil, 0, false
	}
	inst, err := h.db.GetInstanceByToken(token)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
		return nil, 0, false
	}
	iid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || iid != inst.ID {
		http.Error(w, `{"error":"instance_id mismatch"}`, http.StatusForbidden)
		return nil, 0, false
	}
	return inst, iid, true
}

func (h *Handler) authOwner(r *http.Request, instanceID int64) (*db.Instance, bool) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		return nil, false
	}
	inst, err := h.db.GetInstanceByID(instanceID)
	if err != nil || inst == nil || inst.UserID != claims.UserID {
		return nil, false
	}
	return inst, true
}

// --- Owner JWT ---

func (h *Handler) GetAgents(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if _, ok := h.authOwner(r, iid); !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	list, err := h.db.ListInstanceAgents(iid)
	if err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"agents": list,
		"limits": collaborationLimitsPayload(),
	})
}

func (h *Handler) PutAgents(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	inst, ok := h.authOwner(r, iid)
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		Agents []struct {
			AgentSlug   string `json:"agent_slug"`
			DisplayName string `json:"display_name"`
		} `json:"agents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	rows := make([]db.InstanceAgentRow, 0, len(body.Agents))
	for _, a := range body.Agents {
		rows = append(rows, db.InstanceAgentRow{
			AgentSlug:   a.AgentSlug,
			DisplayName: a.DisplayName,
		})
	}
	if err := h.db.ReplaceInstanceAgents(iid, inst.UserID, rows); err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	h.pushTopologyUpdated(iid)
	writeJSON(w, map[string]any{"status": "ok", "limits": collaborationLimitsPayload()})
}

func (h *Handler) GetTopology(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if _, ok := h.authOwner(r, iid); !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	edges, err := h.db.ListTopologyEdges(iid)
	if err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	pairs := make([][2]string, 0, len(edges))
	for _, e := range edges {
		pairs = append(pairs, [2]string{e.AgentSlugLo, e.AgentSlugHi})
	}
	ver, _ := h.db.GetAgentTopologyVersion(iid)
	writeJSON(w, map[string]any{
		"edges":   pairs,
		"version": ver,
		"limits":  collaborationLimitsPayload(),
	})
}

func (h *Handler) PutTopology(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if _, ok := h.authOwner(r, iid); !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		Edges [][2]string `json:"edges"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.ReplaceTopologyEdges(iid, body.Edges); err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	h.pushTopologyUpdated(iid)
	writeJSON(w, map[string]any{"status": "ok", "limits": collaborationLimitsPayload()})
}

func (h *Handler) ListMails(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	if _, ok := h.authOwner(r, iid); !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	h.serveListInternalMails(w, iid, r)
}

// serveListInternalMails 与 JWT ListMails、容器 bridge 共用（query: thread_id, limit, offset）。
func (h *Handler) serveListInternalMails(w http.ResponseWriter, iid int64, r *http.Request) {
	thread := r.URL.Query().Get("thread_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := h.db.ListInternalMails(iid, thread, limit, offset)
	if err != nil {
		if errors.Is(err, db.ErrInternalMailListOffsetTooLarge) || errors.Is(err, db.ErrInternalMailInvalidThreadFilter) {
			writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	total, err := h.db.CountInternalMails(iid, thread)
	if err != nil {
		if errors.Is(err, db.ErrInternalMailInvalidThreadFilter) {
			writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
			return
		}
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"mails":  list,
		"total":  total,
		"limits": collaborationLimitsPayload(),
	})
}

func (h *Handler) PostResolve(w http.ResponseWriter, r *http.Request) {
	iid, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid id"}`, http.StatusBadRequest)
		return
	}
	inst, ok := h.authOwner(r, iid)
	if !ok {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	lim := collaborationLimitsPayload()
	slug, amb, err := h.db.ResolveDisplayNameForInstance(iid, inst.UserID, body.Name)
	if err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(amb) > 0 {
		writeJSON(w, map[string]any{"ok": false, "ambiguous": amb, "limits": lim})
		return
	}
	if slug == "" {
		writeJSON(w, map[string]any{"ok": false, "not_found": true, "limits": lim})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "agent_slug": slug, "limits": lim})
}

// --- Container token ---

func (h *Handler) ContainerListMails(w http.ResponseWriter, r *http.Request) {
	_, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	h.serveListInternalMails(w, iid, r)
}

func (h *Handler) ContainerGetRoster(w http.ResponseWriter, r *http.Request) {
	_, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	list, err := h.db.ListInstanceAgents(iid)
	if err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"agents": list,
		"limits": collaborationLimitsPayload(),
	})
}

func (h *Handler) ContainerGetTopology(w http.ResponseWriter, r *http.Request) {
	_, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	edges, err := h.db.ListTopologyEdges(iid)
	if err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	pairs := make([][2]string, 0, len(edges))
	for _, e := range edges {
		pairs = append(pairs, [2]string{e.AgentSlugLo, e.AgentSlugHi})
	}
	ver, _ := h.db.GetAgentTopologyVersion(iid)
	writeJSON(w, map[string]any{
		"edges":   pairs,
		"version": ver,
		"limits":  collaborationLimitsPayload(),
	})
}

func (h *Handler) ContainerPostResolve(w http.ResponseWriter, r *http.Request) {
	inst, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	lim := collaborationLimitsPayload()
	slug, amb, err := h.db.ResolveDisplayNameForInstance(iid, inst.UserID, body.Name)
	if err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(amb) > 0 {
		writeJSON(w, map[string]any{"ok": false, "ambiguous": amb, "limits": lim})
		return
	}
	if slug == "" {
		writeJSON(w, map[string]any{"ok": false, "not_found": true, "limits": lim})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "agent_slug": slug, "limits": lim})
}

func (h *Handler) ContainerGetMail(w http.ResponseWriter, r *http.Request) {
	_, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	midStr := chi.URLParam(r, "mailId")
	mid, err := strconv.ParseInt(midStr, 10, 64)
	if err != nil || mid < 1 {
		http.Error(w, `{"error":"invalid mail id"}`, http.StatusBadRequest)
		return
	}
	row, err := h.db.GetInternalMailByID(iid, mid)
	if err != nil {
		http.Error(w, `{"error":"db"}`, http.StatusInternalServerError)
		return
	}
	if row == nil {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSONValueWithCollaborationLimits(w, row)
}

func (h *Handler) ContainerPostMail(w http.ResponseWriter, r *http.Request) {
	_, iid, ok := h.authInstance(w, r)
	if !ok {
		return
	}
	var body struct {
		FromSlug   string `json:"from_slug"`
		ToSlug     string `json:"to_slug"`
		Subject    string `json:"subject"`
		Body       string `json:"body"`
		ThreadID   string `json:"thread_id"`
		InReplyTo  *int64 `json:"in_reply_to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if _, err := h.db.ValidateInternalMailThreadForPost(body.ThreadID); err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.db.VerifySlugsBelongToInstance(iid, body.FromSlug, body.ToSlug); err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	neighbor, err := h.db.AreNeighbors(iid, body.FromSlug, body.ToSlug)
	if err != nil || !neighbor {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, "双方在通讯拓扑上不是邻居，无法发送内部邮件")
		return
	}
	if err := h.db.ValidateInternalMailReply(iid, body.ThreadID, body.FromSlug, body.ToSlug, body.InReplyTo); err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	mailID, ver, err := h.db.InsertInternalMail(iid, body.ThreadID, body.FromSlug, body.ToSlug, body.Subject, body.Body, body.InReplyTo)
	if err != nil {
		writeJSONErrorWithLimits(w, http.StatusBadRequest, err.Error())
		return
	}
	h.pushInternalMail(iid, mailID, body.ThreadID, body.FromSlug, body.ToSlug, ver)
	writeJSON(w, map[string]any{
		"ok": true, "id": mailID, "topology_version": ver,
		"limits": collaborationLimitsPayload(),
	})
}

func (h *Handler) pushTopologyUpdated(instanceID int64) {
	ver, err := h.db.GetAgentTopologyVersion(instanceID)
	if err != nil {
		return
	}
	msg := map[string]any{
		"type":    "collab.topology_updated",
		"payload": map[string]any{"version": ver},
	}
	if err := h.hub.WriteContainerJSON(instanceID, msg); err != nil {
		log.Printf("[collab] push topology to instance %d: %v", instanceID, err)
	}
	h.hub.WriteAttachedUserJSON(instanceID, msg)
}

func (h *Handler) pushInternalMail(instanceID, mailID int64, threadID, fromSlug, toSlug string, ver int64) {
	msg := map[string]any{
		"type": "collab.internal_mail",
		"payload": map[string]any{
			"id":                mailID,
			"thread_id":         threadID,
			"from_slug":         fromSlug,
			"to_slug":           toSlug,
			"topology_version":  ver,
		},
	}
	if err := h.hub.WriteContainerJSON(instanceID, msg); err != nil {
		log.Printf("[collab] push mail notify instance %d: %v", instanceID, err)
	}
	h.hub.WriteAttachedUserJSON(instanceID, msg)
}

// writeJSONValueWithCollaborationLimits 将 v 编码为 JSON 对象并并入 limits（与列表/发信等接口一致）。
func writeJSONValueWithCollaborationLimits(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		http.Error(w, `{"error":"encode"}`, http.StatusInternalServerError)
		return
	}
	m["limits"] = collaborationLimitsPayload()
	writeJSON(w, m)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONErrorWithLimits(w http.ResponseWriter, status int, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":  errMsg,
		"limits": collaborationLimitsPayload(),
	})
}
