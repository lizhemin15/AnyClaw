package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/config"
	"github.com/anyclaw/anyclaw-api/internal/db"
)

type HostStore interface {
	ListEnabledHosts() ([]*db.Host, error)
	ListAllHostsWithCredentials() ([]*db.Host, error)
	GetHost(id string) (*db.Host, error)
}

type Scheduler struct {
	apiURL     string
	defaultImg string
	configPath string
	hosts      HostStore
}

func New(apiURL, defaultImage, configPath string, hosts HostStore) *Scheduler {
	if defaultImage == "" {
		defaultImage = "openclaw/openclaw"
	}
	return &Scheduler{apiURL: apiURL, defaultImg: defaultImage, configPath: configPath, hosts: hosts}
}

// Run creates a Docker container on a remote host via SSH and returns (containerID, hostID).
// apiURLOverride: when non-empty, use instead of s.apiURL (e.g. from request Host for auto-detect).
// Workspace is a plain directory at /var/lib/anyclaw/ws-{instanceID}, removed on Stop.
func (s *Scheduler) Run(ctx context.Context, instanceID int64, token string, apiURLOverride string) (containerID, hostID string, err error) {
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
	apiURL := s.apiURL
	if apiURLOverride != "" {
		apiURL = apiURLOverride
	}
	log.Printf("[scheduler] instance %d: using host %q (%s:%d), image=%s, apiURL=%s",
		instanceID, host.Name, host.Addr, host.SSHPort, image, apiURL)

	// Create workspace directory (container runs as uid 1000)
	ensureWorkspace := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; mkdir -p /var/lib/anyclaw && \
		WS="/var/lib/anyclaw/ws-%d" && mkdir -p "$WS" && chown -R 1000:1000 "$WS"`, instanceID)
	if _, err := runSSH(host, ensureWorkspace); err != nil {
		log.Printf("[scheduler] ensure workspace on %s failed: %v", host.Addr, err)
		return "", "", fmt.Errorf("ensure workspace: %w", err)
	}

	defaultModel := "gpt-4o"
	if cfg, err := config.Load(s.configPath); err == nil {
		if m := cfg.GetEnabledModel(); m != "" {
			defaultModel = m
		}
	}
	wsPath := fmt.Sprintf("/var/lib/anyclaw/ws-%d", instanceID)
	containerName := fmt.Sprintf("anyclaw-inst-%d", instanceID)
	cmd := fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; docker run -d --name %s --pull always -v %s:/workspace -e ANYCLAW_AGENTS_DEFAULTS_WORKSPACE=/workspace -e ANYCLAW_AGENTS_DEFAULTS_MODEL_NAME='%s' -e ANYCLAW_API_URL='%s' -e ANYCLAW_INSTANCE_ID=%d -e ANYCLAW_TOKEN='%s' %s 2>&1",
		containerName, wsPath, defaultModel, apiURL, instanceID, token, image)
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
// When hostID is empty, tries ALL hosts (including disabled) until docker rm succeeds.
// When containerID is empty but instanceID > 0, tries removing by name anyclaw-inst-{id}.
func (s *Scheduler) Stop(ctx context.Context, hostID, containerID string, instanceID int64) error {
	rmTarget := containerID
	if rmTarget == "" && instanceID > 0 {
		rmTarget = fmt.Sprintf("anyclaw-inst-%d", instanceID)
		log.Printf("[scheduler] container_id empty, trying remove by name %s", rmTarget)
	}
	if rmTarget == "" {
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
	// Safety: when instanceID is known, always verify container belongs to this instance before rm
	// to avoid accidentally deleting wrong containers (wrong DB data or manual name collision).
	verifyBeforeRm := instanceID > 0
	var lastErr error
	for _, host := range hostsToTry {
		if verifyBeforeRm {
			verifyCmd := fmt.Sprintf("export PATH=/usr/local/bin:/usr/bin:$PATH; docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' %s 2>/dev/null | grep -E '^ANYCLAW_INSTANCE_ID=' || true", rmTarget)
			out, err := runSSH(host, verifyCmd)
			if err != nil {
				lastErr = err
				continue
			}
			expected := fmt.Sprintf("ANYCLAW_INSTANCE_ID=%d", instanceID)
			got := strings.TrimSpace(out)
			if got != expected {
				if got != "" {
					log.Printf("[scheduler] skip rm on %s: container %s env mismatch (got %q, expect %q)", host.Addr, rmTarget, got, expected)
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
		// Remove workspace (plain dir or legacy loop mount)
		if instanceID > 0 {
			cleanup := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH; \
				WS="/var/lib/anyclaw/ws-%d" FILE="/var/lib/anyclaw/ws-%d.img"; \
				mountpoint -q "$WS" 2>/dev/null && umount "$WS"; rm -rf "$WS"; rm -f "$FILE"`, instanceID, instanceID)
			if _, err := runSSH(host, cleanup); err != nil {
				log.Printf("[scheduler] workspace cleanup on %s failed (non-fatal): %v", host.Addr, err)
			}
		}
		return nil
	}
	return lastErr
}
