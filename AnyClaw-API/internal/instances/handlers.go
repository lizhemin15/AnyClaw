package instances

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/energy"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/anyclaw/anyclaw-api/internal/scheduler"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	db        *db.DB
	scheduler *scheduler.Scheduler
	apiURL    string
}

func New(db *db.DB, sched *scheduler.Scheduler, apiURL string) *Handler {
	return &Handler{db: db, scheduler: sched, apiURL: apiURL}
}

type CreateRequest struct {
	Name string `json:"name"`
}

// resolveAPIURL returns the API URL for containers. When config has localhost, use request Host for auto-detect.
func (h *Handler) resolveAPIURL(r *http.Request) string {
	if h.apiURL != "" && !strings.Contains(h.apiURL, "localhost") {
		return ""
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return scheme + "://" + host
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	list, err := h.db.ListInstancesByUserID(claims.UserID)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.Instance{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "instance"
	}
	// 先检查金币，但不扣除；容器真正创建成功后才扣
	u, _ := h.db.GetUserByID(claims.UserID)
	if u == nil || u.Energy < energy.AdoptCost {
		http.Error(w, `{"error":"金币不足，领养需要 100 金币"}`, http.StatusBadRequest)
		return
	}
	token := "inst-" + uuid.New().String()
	inst, err := h.db.CreateInstance(claims.UserID, name, token, 0)
	if err != nil {
		http.Error(w, `{"error":"failed to create instance"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("[instances] creating instance id=%d name=%q, waiting for container", inst.ID, name)
	apiURL := h.resolveAPIURL(r)
	containerID, hostID, err := h.scheduler.Run(context.Background(), inst.ID, token, apiURL)
	if err != nil {
		log.Printf("[instances] scheduler.Run failed for instance %d: %v", inst.ID, err)
		_ = h.db.DeleteInstance(inst.ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "创建宠物失败：" + err.Error()})
		return
	}
	ok, err := h.db.DeductUserEnergy(claims.UserID, energy.AdoptCost)
	if err != nil || !ok {
		log.Printf("[instances] deduct energy failed for user %d, container already running", claims.UserID)
		_ = h.scheduler.Stop(context.Background(), hostID, containerID, inst.ID)
		_ = h.db.DeleteInstance(inst.ID)
		http.Error(w, `{"error":"金币不足或系统异常"}`, http.StatusInternalServerError)
		return
	}
	if err := h.db.UpdateInstanceContainer(inst.ID, containerID, hostID); err != nil {
		_ = h.db.AddUserEnergy(claims.UserID, energy.AdoptCost)
		_ = h.scheduler.Stop(context.Background(), hostID, containerID, inst.ID)
		_ = h.db.DeleteInstance(inst.ID)
		http.Error(w, `{"error":"failed to save instance"}`, http.StatusInternalServerError)
		return
	}
	_ = h.db.AddInstanceEnergy(inst.ID, energy.AdoptCost)
	inst, _ = h.db.GetInstanceByID(inst.ID)
	log.Printf("[instances] instance %d container started: %s on host %s", inst.ID, containerID, hostID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(inst)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid instance id"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(id)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"instance not found"}`, http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inst)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid instance id"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(id)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"instance not found"}`, http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	_ = h.scheduler.Stop(r.Context(), inst.HostID, inst.ContainerID, id)
	_ = h.db.DeleteMessagesByInstance(id)
	if err := h.db.DeleteInstance(id); err != nil {
		http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Feed(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid instance id"}`, http.StatusBadRequest)
		return
	}
	var req struct {
		Amount int `json:"amount"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Amount <= 0 {
		http.Error(w, `{"error":"amount 必须为正整数"}`, http.StatusBadRequest)
		return
	}
	if req.Amount > 1000 {
		http.Error(w, `{"error":"单次喂养最多 1000 活力"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(id)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"宠物不存在"}`, http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	ok, err := h.db.DeductUserEnergy(claims.UserID, req.Amount)
	if err != nil || !ok {
		http.Error(w, `{"error":"金币不足"}`, http.StatusBadRequest)
		return
	}
	if err := h.db.AddInstanceEnergy(id, req.Amount); err != nil {
		_ = h.db.AddUserEnergy(claims.UserID, req.Amount)
		http.Error(w, `{"error":"喂养失败"}`, http.StatusInternalServerError)
		return
	}
	inst, _ = h.db.GetInstanceByID(id)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(inst)
}
