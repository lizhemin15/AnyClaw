package hosts

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
	"github.com/anyclaw/anyclaw-api/internal/scheduler"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var sha256DigestRe = regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)

type Handler struct {
	db                   *db.DB
	checker              StatusChecker
	sched                *scheduler.Scheduler
	apiURL               string
	defaultInstanceImage string
}

type StatusChecker interface {
	CheckHost(host *db.Host) (string, error)
	RunCommand(host *db.Host, cmd string) (string, error)
}

func New(db *db.DB, checker StatusChecker, sched *scheduler.Scheduler, apiURL, defaultInstanceImage string) *Handler {
	if defaultInstanceImage == "" {
		defaultInstanceImage = "jamlily/anyclaw-server:latest"
	}
	return &Handler{db: db, checker: checker, sched: sched, apiURL: apiURL, defaultInstanceImage: defaultInstanceImage}
}

type CreateRequest struct {
	Name             string `json:"name"`
	Addr             string `json:"addr"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHKey           string `json:"ssh_key"`
	SSHPassword      string `json:"ssh_password"`
	DockerImage      string `json:"docker_image"`
	Enabled          bool   `json:"enabled"`
	InstanceCapacity int    `json:"instance_capacity"`
}

type UpdateRequest struct {
	Name             string `json:"name"`
	Addr             string `json:"addr"`
	SSHPort          int    `json:"ssh_port"`
	SSHUser          string `json:"ssh_user"`
	SSHKey           string `json:"ssh_key"`      // empty = keep existing
	SSHPassword      string `json:"ssh_password"` // empty = keep existing
	DockerImage      string `json:"docker_image"`
	Enabled          bool   `json:"enabled"`
	InstanceCapacity int    `json:"instance_capacity"`
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
		ID:               "host-" + uuid.New().String(),
		Name:             req.Name,
		Addr:             req.Addr,
		SSHPort:          req.SSHPort,
		SSHUser:          req.SSHUser,
		SSHKey:           req.SSHKey,
		SSHPassword:      req.SSHPassword,
		DockerImage:      req.DockerImage,
		Enabled:          req.Enabled,
		InstanceCapacity: req.InstanceCapacity,
		Status:           "unknown",
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
	host.InstanceCapacity = req.InstanceCapacity
	if req.SSHPort > 0 {
		host.SSHPort = req.SSHPort
	}
	if req.SSHKey != "" || req.SSHPassword != "" {
		if req.SSHKey != "" {
			host.SSHKey = req.SSHKey
		} else {
			host.SSHKey = ""
		}
		if req.SSHPassword != "" {
			host.SSHPassword = req.SSHPassword
		} else {
			host.SSHPassword = ""
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

// InstanceImageStatusResponse 实例镜像版本检查结果
type InstanceImageStatusResponse struct {
	UpdateAvailable   bool     `json:"update_available"`
	Image             string   `json:"image"`
	CurrentDigest     string   `json:"current_digest,omitempty"`
	LatestDigest      string   `json:"latest_digest,omitempty"`
	InstanceCount     int      `json:"instance_count"`
	InstanceIDs       []int64  `json:"instance_ids,omitempty"`
	Message           string   `json:"message,omitempty"`
}

// InstanceImageStatus 检查宿主机上的实例镜像与 Docker Hub 是否一致
func (h *Handler) InstanceImageStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	image := host.DockerImage
	if image == "" {
		image = h.defaultInstanceImage
	}
	// 确保有 tag
	if !strings.Contains(image, ":") {
		image = image + ":latest"
	}
	instances, _ := h.db.ListRunningInstancesByHostID(id)
	ids := make([]int64, 0, len(instances))
	for _, i := range instances {
		ids = append(ids, i.ID)
	}
	if h.checker == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(InstanceImageStatusResponse{
			Image:         image,
			InstanceCount: len(instances),
			InstanceIDs:   ids,
			Message:       "SSH 未配置",
		})
		return
	}
	// 本地 digest
	out, err := h.checker.RunCommand(host, `docker inspect "`+image+`" --format '{{index .RepoDigests 0}}' 2>/dev/null || echo ''`)
	var localDigest string
	if err == nil && out != "" {
		if m := sha256DigestRe.FindString(out); m != "" {
			localDigest = m
		}
	}
	hubDigest, err := getDockerHubDigest(image)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(InstanceImageStatusResponse{
			Image:         image,
			CurrentDigest: localDigest,
			InstanceCount: len(instances),
			InstanceIDs:   ids,
			Message:       "无法获取 Docker Hub 最新版本: " + err.Error(),
		})
		return
	}
	updateAvailable := localDigest == "" || localDigest != hubDigest
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(InstanceImageStatusResponse{
		UpdateAvailable: updateAvailable,
		Image:           image,
		CurrentDigest:   localDigest,
		LatestDigest:    hubDigest,
		InstanceCount:   len(instances),
		InstanceIDs:     ids,
	})
}

// Drain 排空宿主机：将该主机上所有运行中实例迁移到其他主机
func (h *Handler) Drain(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	if h.sched == nil {
		http.Error(w, `{"error":"scheduler not configured"}`, http.StatusInternalServerError)
		return
	}
	instances, err := h.db.ListRunningInstancesByHostID(id)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if len(instances) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "该主机无运行中实例", "migrated": 0, "failed": 0})
		return
	}
	target, err := h.sched.PickTargetHostForMigration(id)
	if err != nil || target == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "无其他可用宿主机", "migrated": 0, "failed": len(instances)})
		return
	}
	ctx := r.Context()
	var migrated, failed int
	for _, inst := range instances {
		target, err = h.sched.PickTargetHostForMigration(id)
		if err != nil || target == nil {
			log.Printf("[hosts] drain: no target for instance %d", inst.ID)
			failed++
			continue
		}
		cid, newHostID, err := h.sched.MigrateWithInstance(ctx, inst, target.ID, h.apiURL)
		if err != nil {
			log.Printf("[hosts] drain instance %d: %v", inst.ID, err)
			failed++
			continue
		}
		if err := h.db.UpdateInstanceContainer(inst.ID, cid, newHostID); err != nil {
			log.Printf("[hosts] drain instance %d: update container failed: %v", inst.ID, err)
			failed++
			continue
		}
		migrated++
	}
	msg := "排空完成"
	if failed > 0 {
		msg = fmt.Sprintf("迁移 %d 个成功，%d 个失败", migrated, failed)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": failed == 0, "message": msg, "migrated": migrated, "failed": failed})
}

// PullAndRestartInstances 拉取最新镜像并重启该主机上的所有实例
func (h *Handler) PullAndRestartInstances(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	if h.checker == nil || h.sched == nil {
		http.Error(w, `{"error":"SSH 或调度器未配置"}`, http.StatusInternalServerError)
		return
	}
	image := host.DockerImage
	if image == "" {
		image = h.defaultInstanceImage
	}
	if !strings.Contains(image, ":") {
		image = image + ":latest"
	}
	instances, err := h.db.ListRunningInstancesByHostID(id)
	if err != nil {
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	// 1. 拉取最新镜像
	if _, err := h.checker.RunCommand(host, "docker pull "+image); err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "拉取镜像失败: " + err.Error()})
		return
	}
	// 2. 逐个停止并重启
	ctx := r.Context()
	var failed []int64
	for _, inst := range instances {
		if err := h.sched.Stop(ctx, host.ID, inst.ContainerID, inst.ID); err != nil {
			log.Printf("[hosts] stop instance %d failed: %v", inst.ID, err)
			failed = append(failed, inst.ID)
			continue
		}
		_ = h.db.UpdateInstanceStatus(inst.ID, "creating")
		cid, err := h.sched.RunOnHost(ctx, host.ID, inst.ID, inst.Token, h.apiURL)
		if err != nil {
			log.Printf("[hosts] restart instance %d failed: %v", inst.ID, err)
			failed = append(failed, inst.ID)
			_ = h.db.UpdateInstanceStatus(inst.ID, "error")
			continue
		}
		_ = h.db.UpdateInstanceContainer(inst.ID, cid, host.ID)
	}
	msg := "已完成"
	if len(failed) > 0 {
		msg = "部分实例重启失败"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": len(failed) < len(instances), "message": msg, "failed_ids": failed})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg})
}
