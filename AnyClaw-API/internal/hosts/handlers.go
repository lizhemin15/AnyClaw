package hosts

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
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

// HostMetricsResponse 宿主机 CPU、磁盘、内存使用情况
type HostMetricsResponse struct {
	Disk *DiskMetrics `json:"disk,omitempty"`
	Mem  *MemMetrics  `json:"mem,omitempty"`
	Load *LoadMetrics `json:"load,omitempty"`
	Err  string       `json:"error,omitempty"`
}

type DiskMetrics struct {
	Total string  `json:"total"` // e.g. "50G"
	Used  string  `json:"used"`
	Avail string  `json:"avail"`
	Pct   float64 `json:"pct"` // 0-100
}

type MemMetrics struct {
	Total int `json:"total"` // MB
	Used  int `json:"used"`
	Avail int `json:"avail"`
	Pct   int `json:"pct"` // 0-100
}

type LoadMetrics struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// HostMetrics 获取宿主机 CPU、磁盘、内存使用情况（通过 SSH 执行 df/free/loadavg）
func (h *Handler) HostMetrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	resp := HostMetricsResponse{}
	if h.checker == nil {
		resp.Err = "SSH 未配置"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	// 单次 SSH 执行多个命令，用换行分隔输出
	cmd := `df -h / 2>/dev/null | tail -1; free -m 2>/dev/null | awk '/^Mem:/'; cat /proc/loadavg 2>/dev/null`
	out, err := h.checker.RunCommand(host, cmd)
	if err != nil {
		resp.Err = err.Error()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		// df: [fs] size used avail use% [mount] - size/used/avail end with G/M/K
		if len(fields) >= 5 && (strings.HasSuffix(fields[1], "G") || strings.HasSuffix(fields[1], "M") || strings.HasSuffix(fields[1], "K")) {
			pctStr := strings.TrimSuffix(fields[4], "%")
			if pct, e := parseFloat(pctStr); e == nil && pct >= 0 && pct <= 100 {
				resp.Disk = &DiskMetrics{
					Total: fields[1],
					Used:  fields[2],
					Avail: fields[3],
					Pct:   pct,
				}
				continue
			}
		}
		// Mem: total used free shared buff/cache available
		if strings.HasPrefix(line, "Mem:") && len(fields) >= 7 {
			total := parseInt(fields[1])
			used := parseInt(fields[2])
			avail := parseInt(fields[6]) // available (newer free)
			if avail <= 0 && len(fields) >= 4 {
				avail = parseInt(fields[3]) // free (older free)
			}
			if total > 0 {
				resp.Mem = &MemMetrics{
					Total: total,
					Used:  used,
					Avail: avail,
					Pct:   used * 100 / total,
				}
			}
			continue
		}
		// loadavg: 0.50 0.45 0.40 1/234 56789
		if len(fields) >= 3 && !strings.HasPrefix(line, "Mem:") && !strings.HasPrefix(line, "/dev/") {
			if l1, e1 := parseFloat(fields[0]); e1 == nil {
				if l5, e5 := parseFloat(fields[1]); e5 == nil {
					if l15, e15 := parseFloat(fields[2]); e15 == nil {
						resp.Load = &LoadMetrics{Load1: l1, Load5: l5, Load15: l15}
					}
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}

func parseInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
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
// Docker Hub 请求通过 SSH 在宿主机上执行，确保使用宿主机网络（宿主机可访问 Docker Hub）
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
	// 本地 digest（宿主机 SSH）：遍历所有 RepoDigests，多架构/多源镜像可能有多个
	out, err := h.checker.RunCommand(host, `docker inspect "`+image+`" --format '{{range .RepoDigests}}{{println .}}{{end}}' 2>/dev/null || echo ''`)
	localDigests := make(map[string]bool)
	var localDigest string // 取第一个用于展示
	if err == nil && out != "" {
		for _, m := range sha256DigestRe.FindAllString(out, -1) {
			localDigests[m] = true
			if localDigest == "" {
				localDigest = m
			}
		}
	}
	// Docker Hub digests 通过宿主机 SSH 获取（含 manifest list 各平台 digest，与本地 RepoDigests 可比）
	hubDigests, err := getDockerHubDigestsViaHost(h.checker, host, image)
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
	hubDigestSet := make(map[string]bool)
	for _, d := range hubDigests {
		hubDigestSet[d] = true
	}
	// 任一本地 digest 在 hub digests 中即视为已最新（多架构时本地存平台 digest，hub 需解析 list 获取全部）
	hasMatch := false
	for d := range localDigests {
		if hubDigestSet[d] {
			hasMatch = true
			break
		}
	}
	updateAvailable := localDigest == "" || !hasMatch
	hubDigestForResp := ""
	if len(hubDigests) > 0 {
		hubDigestForResp = hubDigests[0]
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(InstanceImageStatusResponse{
		UpdateAvailable: updateAvailable,
		Image:           image,
		CurrentDigest:   localDigest,
		LatestDigest:    hubDigestForResp,
		InstanceCount:   len(instances),
		InstanceIDs:     ids,
	})
}

// pruneImagesOnHost 在宿主机上执行 docker image prune -f 清理悬空镜像
func pruneImagesOnHost(checker StatusChecker, host *db.Host) {
	if checker == nil {
		return
	}
	out, err := checker.RunCommand(host, "export PATH=/usr/local/bin:/usr/bin:$PATH; docker image prune -f")
	if err != nil {
		log.Printf("[hosts] prune images on %s failed: %v", host.Addr, err)
		return
	}
	if strings.TrimSpace(out) != "" {
		log.Printf("[hosts] prune on %s: %s", host.Addr, strings.TrimSpace(out))
	}
}

// PruneImages 在指定宿主机上清理悬空镜像（<none> 的旧版本）
func (h *Handler) PruneImages(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	host, err := h.db.GetHost(id)
	if err != nil || host == nil {
		http.Error(w, `{"error":"host not found"}`, http.StatusNotFound)
		return
	}
	if h.checker == nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "message": "SSH 未配置"})
		return
	}
	pruneImagesOnHost(h.checker, host)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": "已清理悬空镜像"})
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
	if h.checker != nil && migrated > 0 {
		pruneImagesOnHost(h.checker, host)
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
	failedReasons := make(map[int64]string)
	for _, inst := range instances {
		if err := h.sched.Stop(ctx, host.ID, inst.ContainerID, inst.ID, false, true); err != nil {
			log.Printf("[hosts] stop instance %d failed: %v", inst.ID, err)
			failed = append(failed, inst.ID)
			failedReasons[inst.ID] = "停止容器失败: " + err.Error()
			continue
		}
		_ = h.db.UpdateInstanceStatus(inst.ID, "creating")
		cid, err := h.sched.RunOnHost(ctx, host.ID, inst.ID, inst.Token, h.apiURL)
		if err != nil {
			// 重试一次（可能是瞬时网络/资源问题）
			cid, err = h.sched.RunOnHost(ctx, host.ID, inst.ID, inst.Token, h.apiURL)
		}
		if err != nil {
			log.Printf("[hosts] restart instance %d failed: %v", inst.ID, err)
			failed = append(failed, inst.ID)
			failedReasons[inst.ID] = "启动容器失败: " + err.Error()
			_ = h.db.UpdateInstanceStatus(inst.ID, "error")
			continue
		}
		_ = h.db.UpdateInstanceContainer(inst.ID, cid, host.ID)
	}
	// 3. 清理悬空镜像（<none> 的旧版本）
	pruneImagesOnHost(h.checker, host)
	msg := "已完成"
	if len(failed) > 0 {
		msg = "部分实例重启失败"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": len(failed) < len(instances), "message": msg, "failed_ids": failed, "failed_reasons": failedReasons})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "message": msg})
}
