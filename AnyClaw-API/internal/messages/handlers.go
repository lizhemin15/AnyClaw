package messages

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	db *db.DB
}

func New(db *db.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	instanceID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid instance id"}`, http.StatusBadRequest)
		return
	}
	inst, err := h.db.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		http.Error(w, `{"error":"instance not found"}`, http.StatusNotFound)
		return
	}
	if inst.UserID != claims.UserID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}
	var beforeID int64
	if b := r.URL.Query().Get("before"); b != "" {
		beforeID, _ = strconv.ParseInt(b, 10, 64)
	}
	list, err := h.db.ListMessages(instanceID, limit, beforeID)
	if err != nil {
		http.Error(w, `{"error":"failed to load messages"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.Message{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"messages": list})
}
