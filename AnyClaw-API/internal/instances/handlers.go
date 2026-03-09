package instances

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/auth"
	"github.com/anyclaw/anyclaw-api/internal/db"
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

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r.Context())
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
	claims := auth.FromContext(r.Context())
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
	token := "inst-" + uuid.New().String()
	inst, err := h.db.CreateInstance(claims.UserID, name, token)
	if err != nil {
		http.Error(w, `{"error":"failed to create instance"}`, http.StatusInternalServerError)
		return
	}
	go func() {
		containerID, hostID, err := h.scheduler.Run(context.Background(), inst.ID, token)
		if err != nil {
			_ = h.db.UpdateInstanceStatus(inst.ID, "error")
			return
		}
		_ = h.db.UpdateInstanceContainer(inst.ID, containerID, hostID)
	}()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(inst)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	claims := auth.FromContext(r.Context())
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
	claims := auth.FromContext(r.Context())
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
	if inst.ContainerID != "" {
		_ = h.scheduler.Stop(r.Context(), inst.HostID, inst.ContainerID)
	}
	if err := h.db.DeleteInstance(id); err != nil {
		http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
