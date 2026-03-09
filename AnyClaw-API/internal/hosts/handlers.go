package hosts

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	db       *db.DB
	checker  StatusChecker
}

type StatusChecker interface {
	CheckHost(host *db.Host) (string, error)
}

func New(db *db.DB, checker StatusChecker) *Handler {
	return &Handler{db: db, checker: checker}
}

type CreateRequest struct {
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHKey      string `json:"ssh_key"`
	SSHPassword string `json:"ssh_password"`
	DockerImage string `json:"docker_image"`
	Enabled     bool   `json:"enabled"`
}

type UpdateRequest struct {
	Name        string `json:"name"`
	Addr        string `json:"addr"`
	SSHPort     int    `json:"ssh_port"`
	SSHUser     string `json:"ssh_user"`
	SSHKey      string `json:"ssh_key"`      // empty = keep existing
	SSHPassword string `json:"ssh_password"` // empty = keep existing
	DockerImage string `json:"docker_image"`
	Enabled     bool   `json:"enabled"`
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.db.ListHosts()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.Host{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Addr = strings.TrimSpace(req.Addr)
	req.SSHUser = strings.TrimSpace(req.SSHUser)
	if req.Name == "" || req.Addr == "" || req.SSHUser == "" {
		http.Error(w, `{"error":"name, addr, ssh_user required"}`, http.StatusBadRequest)
		return
	}
	if req.SSHKey == "" && req.SSHPassword == "" {
		http.Error(w, `{"error":"ssh_key or ssh_password required"}`, http.StatusBadRequest)
		return
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	host := &db.Host{
		ID:          "host-" + uuid.New().String(),
		Name:        req.Name,
		Addr:        req.Addr,
		SSHPort:     req.SSHPort,
		SSHUser:     req.SSHUser,
		SSHKey:      req.SSHKey,
		SSHPassword: req.SSHPassword,
		DockerImage: req.DockerImage,
		Enabled:     req.Enabled,
		Status:      "unknown",
	}
	if err := h.db.CreateHost(host); err != nil {
		http.Error(w, `{"error":"failed to create host"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(host)
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(host)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	var req UpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	host.Name = strings.TrimSpace(req.Name)
	host.Addr = strings.TrimSpace(req.Addr)
	host.SSHUser = strings.TrimSpace(req.SSHUser)
	host.DockerImage = strings.TrimSpace(req.DockerImage)
	host.Enabled = req.Enabled
	if req.SSHPort > 0 {
		host.SSHPort = req.SSHPort
	}
	if req.SSHKey != "" || req.SSHPassword != "" {
		if req.SSHKey != "" {
			host.SSHKey = req.SSHKey
			host.SSHPassword = ""
		} else {
			host.SSHPassword = req.SSHPassword
			host.SSHKey = ""
		}
		if err := h.db.UpdateHost(host); err != nil {
			http.Error(w, `{"error":"failed to update"}`, http.StatusInternalServerError)
			return
		}
	} else {
		if err := h.db.UpdateHostNoKey(host); err != nil {
			http.Error(w, `{"error":"failed to update"}`, http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(host)
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.db.DeleteHost(id); err != nil {
		http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CheckStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	status := "unknown"
	if h.checker != nil {
		if s, err := h.checker.CheckHost(host); err == nil {
			status = s
		} else {
			status = "error"
		}
	}
	_ = h.db.UpdateHostStatus(id, status)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}
