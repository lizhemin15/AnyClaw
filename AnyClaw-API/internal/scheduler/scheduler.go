package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
)

type HostStore interface {
	ListEnabledHosts() ([]*db.Host, error)
	GetHost(id string) (*db.Host, error)
}

type Scheduler struct {
	apiURL     string
	defaultImg string
	hosts      HostStore
}

func New(apiURL, defaultImage string, hosts HostStore) *Scheduler {
	if defaultImage == "" {
		defaultImage = "anyclaw/anyclaw"
	}
	return &Scheduler{apiURL: apiURL, defaultImg: defaultImage, hosts: hosts}
}

// Run creates a Docker container on a remote host via SSH and returns (containerID, hostID).
// Workspace is persisted via a 1GB loop-mounted filesystem at /var/lib/anyclaw/ws-{instanceID}.
func (s *Scheduler) Run(ctx context.Context, instanceID int64, token string) (containerID, hostID string, err error) {
	list, err := s.hosts.ListEnabledHosts()
	if err != nil {
		return "", "", fmt.Errorf("list hosts: %w", err)
	}
	if len(list) == 0 {
		log.Printf("[scheduler] no enabled hosts in DB - add a host at /admin/hosts with enabled=true")
		return "", "", fmt.Errorf("no enabled hosts configured")
	}
	host := list[0]
	image := host.DockerImage
	if image == "" {
		image = s.defaultImg
	}
	log.Printf("[scheduler] instance %d: using host %q (%s:%d), image=%s, apiURL=%s",
		instanceID, host.Name, host.Addr, host.SSHPort, image, s.apiURL)
	if strings.Contains(s.apiURL, "localhost") && host.Addr != "127.0.0.1" && host.Addr != "localhost" {
		log.Printf("[scheduler] 警告: apiURL 为 localhost，容器在远程 Host 上无法访问。请配置 ANYCLAW_API_URL 为公网地址")
	}

	// Ensure 1GB workspace volume exists (loop device) and is mounted
	ensureWorkspace := fmt.Sprintf(`mkdir -p /var/lib/anyclaw && \
		FILE="/var/lib/anyclaw/ws-%d.img" && \
		MOUNT="/var/lib/anyclaw/ws-%d" && \
		if [ ! -f "$FILE" ]; then \
			dd if=/dev/zero of="$FILE" bs=1M count=0 seek=1024 2>/dev/null && \
			mkfs.ext4 -F "$FILE" >/dev/null 2>&1 && \
			mkdir -p "$MOUNT" && mount -o loop "$FILE" "$MOUNT"; \
		elif ! mountpoint -q "$MOUNT" 2>/dev/null; then \
			mkdir -p "$MOUNT" && mount -o loop "$FILE" "$MOUNT"; \
		fi`, instanceID, instanceID)
	if _, err := runSSH(host, ensureWorkspace); err != nil {
		log.Printf("[scheduler] ensure workspace on %s failed: %v", host.Addr, err)
		return "", "", fmt.Errorf("ensure workspace: %w", err)
	}

	mountPath := fmt.Sprintf("/var/lib/anyclaw/ws-%d", instanceID)
	cmd := fmt.Sprintf("docker run -d --pull always -v %s:/workspace -e PICOCLAW_AGENTS_DEFAULTS_WORKSPACE=/workspace -e ANYCLAW_API_URL='%s' -e ANYCLAW_INSTANCE_ID=%d -e ANYCLAW_TOKEN='%s' %s 2>&1",
		mountPath, s.apiURL, instanceID, token, image)
	out, err := runSSH(host, cmd)
	if err != nil {
		log.Printf("[scheduler] ssh docker run on %s failed: %v", host.Addr, err)
		return "", "", err
	}
	containerID = strings.TrimSpace(out)
	return containerID, host.ID, nil
}

// Stop stops and removes a container on the given host.
// If instanceID > 0, also unmounts and removes the workspace volume.
func (s *Scheduler) Stop(ctx context.Context, hostID, containerID string, instanceID int64) error {
	if containerID == "" {
		return nil
	}
	if hostID == "" {
		log.Printf("[scheduler] skip stop: host_id empty for container %s", containerID)
		return nil
	}
	host, err := s.hosts.GetHost(hostID)
	if err != nil || host == nil {
		return fmt.Errorf("host not found: %s", hostID)
	}
	cmd := fmt.Sprintf("docker rm -f %s", containerID)
	if _, err := runSSH(host, cmd); err != nil {
		log.Printf("[scheduler] ssh docker rm on %s failed: %v", host.Addr, err)
		return err
	}
	// Unmount and remove workspace volume when instance is deleted
	if instanceID > 0 {
		cleanup := fmt.Sprintf(`MOUNT="/var/lib/anyclaw/ws-%d" && FILE="/var/lib/anyclaw/ws-%d.img" && \
			if mountpoint -q "$MOUNT" 2>/dev/null; then umount "$MOUNT"; fi && \
			rm -f "$FILE"`, instanceID, instanceID)
		if _, err := runSSH(host, cleanup); err != nil {
			log.Printf("[scheduler] workspace cleanup on %s failed (non-fatal): %v", host.Addr, err)
		}
	}
	return nil
}
