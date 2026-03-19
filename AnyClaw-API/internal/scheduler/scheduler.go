package scheduler

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
)

// shellEscapeSingleQuoted 转义单引号，使值在 shell 单引号内安全。
func shellEscapeSingleQuoted(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

// buildDockerRunCmd 构建统一的 docker run 命令，新建、重启、迁移等操作均使用此配置，保证挂载与环境变量一致。
// ANYCLAW_VOICE_API_KEY/BASE 供 ASR（语音识别，ChatAnywhere + Groq 均支持）。
// ANYCLAW_TTS_API_KEY/BASE  供 TTS（语音合成，仅 ChatAnywhere 等支持，Groq 不支持）。
func buildDockerRunCmd(containerName, wsPath, image, defaultModel, apiURL string, instanceID int64, token string, voiceAPIKey, voiceAPIBase, ttsAPIKey, ttsAPIBase string) string {
	voiceEnv := ""
	if voiceAPIKey != "" {
		voiceEnv = fmt.Sprintf(" -e ANYCLAW_VOICE_API_KEY='%s'", shellEscapeSingleQuoted(voiceAPIKey))
		if voiceAPIBase != "" {
			voiceEnv += fmt.Sprintf(" -e ANYCLAW_VOICE_API_BASE='%s'", shellEscapeSingleQuoted(voiceAPIBase))
		}
	}
	if ttsAPIKey != "" {
		voiceEnv += fmt.Sprintf(" -e ANYCLAW_TTS_API_KEY='%s'", shellEscapeSingleQuoted(ttsAPIKey))
		if ttsAPIBase != "" {
			voiceEnv += fmt.Sprintf(" -e ANYCLAW_TTS_API_BASE='%s'", shellEscapeSingleQuoted(ttsAPIBase))
		}
	}
	return fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; docker run -d --name %s --pull always -v %s:/workspace -e TZ=Asia/Shanghai -e ANYCLAW_CONFIG=/workspace/config.json -e ANYCLAW_AGENTS_DEFAULTS_WORKSPACE=/workspace -e ANYCLAW_AGENTS_DEFAULTS_MODEL_NAME='%s' -e ANYCLAW_API_URL='%s' -e ANYCLAW_INSTANCE_ID=%d -e ANYCLAW_TOKEN='%s'%s %s gateway 2>&1",
		containerName, wsPath, shellEscapeSingleQuoted(defaultModel), shellEscapeSingleQuoted(apiURL), instanceID, shellEscapeSingleQuoted(token), voiceEnv, image)
}

// isGroqEndpoint 判断给定 endpoint 是否为 Groq（Groq 不支持 TTS）。
func isGroqEndpoint(endpoint string) bool {
	return strings.Contains(strings.ToLower(endpoint), "groq.com")
}

// extractContainerID 从 docker run -d 输出中提取 64 位容器 ID（避免 pull 进度等导致 Data too long）
func extractContainerID(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		s := strings.TrimSpace(lines[i])
		if s == "" {
			continue
		}
		// 取第一个 64 位 hex 串（Docker 容器 ID）
		if m := regexp.MustCompile(`[a-f0-9]{64}`).FindString(s); m != "" {
			return m
		}
		// 若整行是短 ID（12 位）或 64 位，直接取
		if len(s) <= 64 && regexp.MustCompile(`^[a-f0-9]+$`).MatchString(s) {
			return s
		}
	}
	// 兜底：取最后一行前 64 字符
	last := strings.TrimSpace(lines[len(lines)-1])
	if len(last) > 64 {
		return last[:64]
	}
	return last
}

type HostStore interface {
	ListEnabledHosts() ([]*db.Host, error)
	ListAllHostsWithCredentials() ([]*db.Host, error)
	GetHost(id string) (*db.Host, error)
	CountRunningInstancesByHostID(hostID string) (int, error)
}


type Scheduler struct {
	apiURL     string
	defaultImg string
	configPath string
	hosts      HostStore
}

func New(apiURL, defaultImage, configPath string, hosts HostStore) *Scheduler {
	if defaultImage == "" {
		defaultImage = "anyclaw/anyclaw"
	}
	return &Scheduler{apiURL: apiURL, defaultImg: defaultImage, configPath: configPath, hosts: hosts}
}

// Run creates a Docker container on a remote host via SSH and returns (containerID, hostID).
// 负载均衡：选择当前实例数最少的宿主机。
// apiURLOverride: when non-empty, use instead of s.apiURL (e.g. from request Host for auto-detect).
func (s *Scheduler) Run(ctx context.Context, instanceID int64, token string, apiURLOverride string) (containerID, hostID string, err error) {
	list, err := s.hosts.ListEnabledHosts()
	if err != nil {
		return "", "", fmt.Errorf("list hosts: %w", err)
	}
	if len(list) == 0 {
		log.Printf("[scheduler] no enabled hosts in DB - add a host at /admin/hosts with enabled=true")
		return "", "", fmt.Errorf("no enabled hosts configured")
	}
	// 按实例数升序，选择负载最低的宿主机；排除已达容量上限的主机
	list = s.filterByCapacity(list)
	if len(list) == 0 {
		return "", "", fmt.Errorf("all hosts at capacity")
	}
	host := s.pickLeastLoadedHost(list)
	log.Printf("[scheduler] load balance: picked host %q (instance count: %d)", host.Name, s.hostInstanceCount(host.ID))
	return s.runOnHost(ctx, host, instanceID, token, apiURLOverride)
}

func (s *Scheduler) hostInstanceCount(hostID string) int {
	n, _ := s.hosts.CountRunningInstancesByHostID(hostID)
	return n
}

// filterByCapacity 排除已达实例容量上限的宿主机（capacity 0 表示不限）
func (s *Scheduler) filterByCapacity(hosts []*db.Host) []*db.Host {
	var out []*db.Host
	for _, h := range hosts {
		cap := h.InstanceCapacity
		if cap == 0 {
			out = append(out, h)
			continue
		}
		if s.hostInstanceCount(h.ID) < cap {
			out = append(out, h)
		}
	}
	return out
}

func (s *Scheduler) pickLeastLoadedHost(hosts []*db.Host) *db.Host {
	if len(hosts) == 0 {
		return nil
	}
	if len(hosts) == 1 {
		return hosts[0]
	}
	type hostWithCount struct {
		host  *db.Host
		count int
	}
	withCounts := make([]hostWithCount, len(hosts))
	for i, h := range hosts {
		withCounts[i] = hostWithCount{host: h, count: s.hostInstanceCount(h.ID)}
	}
	sort.Slice(withCounts, func(i, j int) bool {
		return withCounts[i].count < withCounts[j].count
	})
	return withCounts[0].host
}

// PickTargetHostForMigration 返回最适合迁移目标的主机（排除指定主机，选择实例数最少且未达容量上限的）
func (s *Scheduler) PickTargetHostForMigration(excludeHostID string) (*db.Host, error) {
	list, err := s.hosts.ListEnabledHosts()
	if err != nil {
		return nil, err
	}
	var filtered []*db.Host
	for _, h := range list {
		if h.ID != excludeHostID {
			filtered = append(filtered, h)
		}
	}
	filtered = s.filterByCapacity(filtered)
	if len(filtered) == 0 {
		return nil, fmt.Errorf("no other enabled hosts with capacity")
	}
	return s.pickLeastLoadedHost(filtered), nil
}

// RunOnHost 在指定宿主机上创建实例容器，用于拉取最新镜像后重启
func (s *Scheduler) RunOnHost(ctx context.Context, hostID string, instanceID int64, token string, apiURL string) (containerID string, err error) {
	host, err := s.hosts.GetHost(hostID)
	if err != nil || host == nil {
		return "", fmt.Errorf("host not found")
	}
	if host.SSHKey == "" && host.SSHPassword == "" {
		return "", fmt.Errorf("host has no SSH credentials")
	}
	cid, _, err := s.runOnHost(ctx, host, instanceID, token, apiURL)
	return cid, err
}

func (s *Scheduler) runOnHost(ctx context.Context, host *db.Host, instanceID int64, token string, apiURLOverride string) (containerID, hostID string, err error) {
	image := host.DockerImage
	if image == "" {
		image = s.defaultImg
	}
	apiURL := s.apiURL
	if apiURLOverride != "" {
		apiURL = apiURLOverride
	}
	workspaceSizeGB := 0
	if cfg, err := config.Load(s.configPath); err == nil {
		workspaceSizeGB = config.GetWorkspaceSizeGB(cfg)
	}
	log.Printf("[scheduler] instance %d: using host %q (%s:%d), image=%s, apiURL=%s, workspace_size_gb=%d",
		instanceID, host.Name, host.Addr, host.SSHPort, image, apiURL, workspaceSizeGB)

	// Create workspace: size>0 用 loop+ext4 限制存储，否则用普通目录
	var ensureWorkspace string
	if workspaceSizeGB > 0 {
		ensureWorkspace = fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; \
			mkdir -p /var/lib/anyclaw && \
			WS="/var/lib/anyclaw/ws-%d" FILE="/var/lib/anyclaw/ws-%d.img" SIZE=%d && \
			if [ ! -f "$FILE" ]; then truncate -s ${SIZE}G "$FILE" && mkfs.ext4 -F "$FILE"; fi && \
			mkdir -p "$WS" && (mountpoint -q "$WS" || mount -o loop "$FILE" "$WS") && chown -R 1000:1000 "$WS"`,
			instanceID, instanceID, workspaceSizeGB)
	} else {
		ensureWorkspace = fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; mkdir -p /var/lib/anyclaw && \
			WS="/var/lib/anyclaw/ws-%d" && mkdir -p "$WS" && chown -R 1000:1000 "$WS"`, instanceID)
	}
	if _, err := runSSH(host, ensureWorkspace); err != nil {
		log.Printf("[scheduler] ensure workspace on %s failed: %v", host.Addr, err)
		return "", "", fmt.Errorf("ensure workspace: %w", err)
	}

	defaultModel := "gpt-4o"
	voiceAPIKey := ""
	voiceAPIBase := ""
	ttsAPIKey := ""
	ttsAPIBase := ""
	if cfg, err := config.Load(s.configPath); err == nil {
		if m := cfg.GetEnabledModel(); m != "" {
			defaultModel = m
		}
		// Pass the first enabled voice API endpoint for ASR (both ChatAnywhere and Groq support ASR).
		// Only pass TTS credentials for endpoints that support TTS (i.e. not Groq).
		for _, ep := range cfg.VoiceAPI {
			if !ep.Enabled || ep.APIKey == "" {
				continue
			}
			if voiceAPIKey == "" {
				voiceAPIKey = ep.APIKey
				voiceAPIBase = ep.Endpoint
			}
			if ttsAPIKey == "" && !isGroqEndpoint(ep.Endpoint) {
				ttsAPIKey = ep.APIKey
				ttsAPIBase = ep.Endpoint
			}
		}
	}
	wsPath := fmt.Sprintf("/var/lib/anyclaw/ws-%d", instanceID)
	containerName := fmt.Sprintf("anyclaw-inst-%d", instanceID)
	cmd := buildDockerRunCmd(containerName, wsPath, image, defaultModel, apiURL, instanceID, token, voiceAPIKey, voiceAPIBase, ttsAPIKey, ttsAPIBase)
	out, err := runSSH(host, cmd)
	if err != nil {
		log.Printf("[scheduler] ssh docker run on %s failed: %v", host.Addr, err)
		return "", "", err
	}
	// docker run -d 可能输出 pull 进度，容器 ID 在最后一行；只取 64 位 hex 避免 Data too long
	containerID = extractContainerID(out)
	return containerID, host.ID, nil
}

// Stop stops and removes a container on the given host.
// If instanceID > 0 and removeWorkspace is true, also unmounts and removes the workspace volume.
// When removeWorkspace is false (e.g. PullAndRestart, Migrate), workspace is preserved for reuse.
// When hostID is empty, tries ALL hosts (including disabled) until docker rm succeeds.
// skipVerify: when true (e.g. PullAndRestart from ListRunningInstancesByHostID), skip env check to allow old/legacy containers.
func (s *Scheduler) Stop(ctx context.Context, hostID, containerID string, instanceID int64, removeWorkspace bool, skipVerify bool) error {
	containerName := fmt.Sprintf("anyclaw-inst-%d", instanceID)
	rmTarget := ""
	if instanceID > 0 {
		rmTarget = containerName
		log.Printf("[scheduler] Stop instance %d: removing container %s (host=%q)", instanceID, rmTarget, hostID)
	} else if containerID != "" {
		rmTarget = strings.TrimSpace(strings.Split(containerID, "\n")[0])
	}
	if rmTarget == "" {
		log.Printf("[scheduler] Stop: no rm target (instanceID=%d containerID=%q)", instanceID, containerID)
		return nil
	}
	allHosts, err := s.hosts.ListAllHostsWithCredentials()
	if err != nil || len(allHosts) == 0 {
		log.Printf("[scheduler] skip stop: no hosts for rm target %s", rmTarget)
		return nil
	}
	var hostsToTry []*db.Host
	if hostID != "" {
		if host, err := s.hosts.GetHost(hostID); err == nil && host != nil && (host.SSHKey != "" || host.SSHPassword != "") {
			hostsToTry = []*db.Host{host}
			for _, h := range allHosts {
				if h.ID != hostID {
					hostsToTry = append(hostsToTry, h)
				}
			}
		}
	}
	if len(hostsToTry) == 0 {
		hostsToTry = allHosts
		log.Printf("[scheduler] host_id empty or invalid, trying all %d hosts for rm target %s", len(allHosts), rmTarget)
	}
	// Safety: when instanceID is known and not skipVerify, verify container belongs to this instance before rm.
	// Lenient parse: accept "ANYCLAW_INSTANCE_ID=5" or "ANYCLAW_INSTANCE_ID=\"5\"" (old Docker/env formats).
	verifyBeforeRm := instanceID > 0 && !skipVerify
	var lastErr error
	for _, host := range hostsToTry {
		if verifyBeforeRm {
			verifyCmd := fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' %s 2>/dev/null | grep -E '^ANYCLAW_INSTANCE_ID=' || true", rmTarget)
			out, err := runSSH(host, verifyCmd)
			if err != nil {
				lastErr = err
				continue
			}
			line := strings.TrimSpace(out)
			verified := false
			if strings.HasPrefix(line, "ANYCLAW_INSTANCE_ID=") {
				valStr := strings.Trim(strings.TrimPrefix(line, "ANYCLAW_INSTANCE_ID="), "\" ")
				if v, e := strconv.ParseInt(valStr, 10, 64); e == nil && v == instanceID {
					verified = true
				}
			}
			if !verified {
				if line != "" {
					log.Printf("[scheduler] skip rm on %s: container %s env mismatch (got %q, expect ANYCLAW_INSTANCE_ID=%d)", host.Addr, rmTarget, line, instanceID)
					lastErr = fmt.Errorf("container %s does not belong to instance %d", rmTarget, instanceID)
				}
				continue
			}
		}
		cmd := fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; docker rm -f %s 2>&1", rmTarget)
		if _, err := runSSH(host, cmd); err != nil {
			log.Printf("[scheduler] docker rm on %s (%s) failed: %v", host.Name, host.Addr, err)
			lastErr = err
			continue
		}
		log.Printf("[scheduler] container %s removed on host %s", rmTarget, host.ID)
		// Remove workspace only when requested (e.g. Delete); preserve for PullAndRestart/Migrate
		if instanceID > 0 && removeWorkspace {
			cleanup := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; \
				WS="/var/lib/anyclaw/ws-%d" FILE="/var/lib/anyclaw/ws-%d.img"; \
				(mountpoint -q "$WS" 2>/dev/null && umount "$WS") || true; \
				rm -rf "$WS" || true; rm -f "$FILE" || true`, instanceID, instanceID)
			if _, err := runSSH(host, cleanup); err != nil {
				log.Printf("[scheduler] workspace cleanup on %s failed (non-fatal): %v", host.Addr, err)
			}
		}
		return nil
	}
	return lastErr
}

// MigrateWithInstance 将实例迁移到目标宿主机，返回新的 containerID 和 hostID
func (s *Scheduler) MigrateWithInstance(ctx context.Context, inst *db.Instance, targetHostID string, apiURL string) (containerID, hostID string, err error) {
	if inst == nil || inst.HostID == "" {
		return "", "", fmt.Errorf("instance or host_id invalid")
	}
	if targetHostID == inst.HostID {
		return "", "", fmt.Errorf("target host is same as current")
	}
	sourceHost, err := s.hosts.GetHost(inst.HostID)
	if err != nil || sourceHost == nil {
		return "", "", fmt.Errorf("source host not found")
	}
	targetHost, err := s.hosts.GetHost(targetHostID)
	if err != nil || targetHost == nil {
		return "", "", fmt.Errorf("target host not found")
	}
	if !targetHost.Enabled {
		return "", "", fmt.Errorf("target host is disabled")
	}
	if targetHost.SSHKey == "" && targetHost.SSHPassword == "" {
		return "", "", fmt.Errorf("target host has no SSH credentials")
	}
	// 1. 停止源主机上的容器（保留工作区，后续 tar 并显式清理）
	if err := s.Stop(ctx, inst.HostID, inst.ContainerID, inst.ID, false, false); err != nil {
		return "", "", fmt.Errorf("stop source container: %w", err)
	}
	// 2. 在源主机上打包工作区内容并流式传输到目标
	wsDir := fmt.Sprintf("/var/lib/anyclaw/ws-%d", inst.ID)
	tarCmd := fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; tar czf - -C %s . 2>/dev/null || true", wsDir)
	src, err := runSSHStreamOut(sourceHost, tarCmd)
	if err != nil {
		return "", "", fmt.Errorf("tar on source: %w", err)
	}
	defer src.Close()
	// 3. 在目标主机上准备目录并解压
	workspaceSizeGB := 0
	if cfg, err := config.Load(s.configPath); err == nil {
		workspaceSizeGB = config.GetWorkspaceSizeGB(cfg)
	}
	var ensureWorkspace string
	if workspaceSizeGB > 0 {
		ensureWorkspace = fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; \
			mkdir -p /var/lib/anyclaw && \
			WS="/var/lib/anyclaw/ws-%d" FILE="/var/lib/anyclaw/ws-%d.img" SIZE=%d && \
			if [ ! -f "$FILE" ]; then truncate -s ${SIZE}G "$FILE" && mkfs.ext4 -F "$FILE"; fi && \
			mkdir -p "$WS" && (mountpoint -q "$WS" || mount -o loop "$FILE" "$WS") && chown -R 1000:1000 "$WS"`,
			inst.ID, inst.ID, workspaceSizeGB)
	} else {
		ensureWorkspace = fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; mkdir -p /var/lib/anyclaw && \
			WS="/var/lib/anyclaw/ws-%d" && mkdir -p "$WS" && chown -R 1000:1000 "$WS"`, inst.ID)
	}
	if _, err := runSSH(targetHost, ensureWorkspace); err != nil {
		return "", "", fmt.Errorf("ensure workspace on target: %w", err)
	}
	extractCmd := fmt.Sprintf("tar xzf - -C /var/lib/anyclaw/ws-%d", inst.ID)
	if err := runSSHStreamIn(targetHost, extractCmd, src); err != nil {
		return "", "", fmt.Errorf("extract on target: %w", err)
	}
	// 4. 在目标主机上启动容器
	cid, newHostID, err := s.runOnHost(ctx, targetHost, inst.ID, inst.Token, apiURL)
	if err != nil {
		return "", "", fmt.Errorf("run on target: %w", err)
	}
	// 5. 清理源主机工作区
	cleanup := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; \
		WS="/var/lib/anyclaw/ws-%d" FILE="/var/lib/anyclaw/ws-%d.img"; \
		(mountpoint -q "$WS" 2>/dev/null && umount "$WS") || true; \
		rm -rf "$WS" || true; rm -f "$FILE" || true`, inst.ID, inst.ID)
	_, _ = runSSH(sourceHost, cleanup)
	return cid, newHostID, nil
}
