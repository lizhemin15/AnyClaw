package instances

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/request"
	"github.com/anyclaw/anyclaw-api/internal/scheduler"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	db          *db.DB
	scheduler   *scheduler.Scheduler
	apiURL      string
	configPath  string
}

func New(db *db.DB, sched *scheduler.Scheduler, apiURL, configPath string) *Handler {
	return &Handler{db: db, scheduler: sched, apiURL: apiURL, configPath: configPath}
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
	cfg, _ := config.Load(h.configPath)
	ec := config.GetEnergyConfig(cfg)
	u, _ := h.db.GetUserByID(claims.UserID)
	if u == nil || u.Energy < ec.AdoptCost {
		http.Error(w, `{"error":"金币不足，领养需要 `+strconv.Itoa(ec.AdoptCost)+` 金币"}`, http.StatusBadRequest)
		return
	}
	token := "inst-" + uuid.New().String()
	inst, err := h.db.CreateInstance(claims.UserID, name, token, 0, 0)
	if err != nil {
		http.Error(w, `{"error":"failed to create instance"}`, http.StatusInternalServerError)
		return
	}
	log.Printf("[instances] creating instance id=%d name=%q, waiting for container", inst.ID, name)
	apiURL := h.resolveAPIURL(r)
	if apiURL == "" {
		if cfg, _ := config.Load(h.configPath); cfg != nil && cfg.APIURL != "" {
			apiURL = strings.TrimSpace(cfg.APIURL)
		}
	}
	if apiURL == "" {
		apiURL = h.apiURL
	}
	containerID, hostID, err := h.scheduler.Run(context.Background(), inst.ID, token, apiURL)
	if err != nil {
		log.Printf("[instances] scheduler.Run failed for instance %d: %v", inst.ID, err)
		_ = h.db.DeleteInstance(inst.ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "创建宠物失败：" + err.Error()})
		return
	}
	ok, err := h.db.DeductUserEnergy(claims.UserID, ec.AdoptCost)
	if err != nil || !ok {
		log.Printf("[instances] deduct energy failed for user %d, container already running", claims.UserID)
		_ = h.scheduler.Stop(context.Background(), hostID, containerID, inst.ID)
		_ = h.db.DeleteInstance(inst.ID)
		http.Error(w, `{"error":"金币不足或系统异常"}`, http.StatusInternalServerError)
		return
	}
	var updateErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*100) * time.Millisecond)
		}
		updateErr = h.db.UpdateInstanceContainer(inst.ID, containerID, hostID)
		if updateErr == nil {
			break
		}
		log.Printf("[instances] UpdateInstanceContainer attempt %d failed for instance %d: %v", attempt+1, inst.ID, updateErr)
	}
	if updateErr != nil {
		_ = h.db.AddUserEnergy(claims.UserID, ec.AdoptCost)
		_ = h.scheduler.Stop(context.Background(), hostID, containerID, inst.ID)
		_ = h.db.DeleteInstance(inst.ID)
		log.Printf("[instances] failed to save instance %d after retries: %v", inst.ID, updateErr)
		http.Error(w, `{"error":"failed to save instance"}`, http.StatusInternalServerError)
		return
	}
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

func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
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
	if err := h.db.UpdateInstanceLastRead(id, claims.UserID); err != nil {
		http.Error(w, `{"error":"failed to mark read"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if err := h.scheduler.Stop(r.Context(), inst.HostID, inst.ContainerID, id); err != nil {
		log.Printf("[instances] Stop failed for instance %d (container may still run): %v", id, err)
	}
	_ = h.db.DeleteMessagesByInstance(id)
	if err := h.db.DeleteInstance(id); err != nil {
		http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	list, err := h.db.ListAllInstancesAdmin()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []*db.AdminInstance{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

func (h *Handler) AdminReconnect(w http.ResponseWriter, r *http.Request) {
	claims := request.FromContext(r.Context())
	if claims == nil || claims.Role != "admin" {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	list, err := h.db.ListRunningInstances()
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	cfg, _ := config.Load(h.configPath)
	apiURL := strings.TrimSpace(cfg.APIURL)
	if apiURL == "" {
		apiURL = h.apiURL
	}
	if apiURL == "" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "请先在 AI 配置中设置 API 地址"})
		return
	}
	reconnected := 0
	for _, inst := range list {
		if err := h.scheduler.Stop(r.Context(), inst.HostID, inst.ContainerID, inst.ID); err != nil {
			log.Printf("[instances] reconnect: stop instance %d failed: %v", inst.ID, err)
			continue
		}
		containerID, hostID, err := h.scheduler.Run(r.Context(), inst.ID, inst.Token, apiURL)
		if err != nil {
			log.Printf("[instances] reconnect: run instance %d failed: %v", inst.ID, err)
			continue
		}
		_ = h.db.UpdateInstanceContainer(inst.ID, containerID, hostID)
		reconnected++
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "已重新连接", "reconnected": reconnected})
}

func (h *Handler) AdminDelete(w http.ResponseWriter, r *http.Request) {
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
	if err := h.scheduler.Stop(r.Context(), inst.HostID, inst.ContainerID, id); err != nil {
		log.Printf("[instances] Admin Stop failed for instance %d: %v", id, err)
	}
	_ = h.db.DeleteMessagesByInstance(id)
	if err := h.db.DeleteInstance(id); err != nil {
		http.Error(w, `{"error":"failed to delete"}`, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

