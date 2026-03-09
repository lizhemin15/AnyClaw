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
func (s *Scheduler) Run(ctx context.Context, instanceID int64, token string) (containerID, hostID string, err error) {
	list, err := s.hosts.ListEnabledHosts()
	if err != nil {
		return "", "", fmt.Errorf("list hosts: %w", err)
	}
	if len(list) == 0 {
		return "", "", fmt.Errorf("no enabled hosts configured")
	}
	host := list[0]
	image := host.DockerImage
	if image == "" {
		image = s.defaultImg
	}
	cmd := fmt.Sprintf("docker run -d -e ANYCLAW_API_URL='%s' -e ANYCLAW_INSTANCE_ID=%d -e ANYCLAW_TOKEN='%s' %s",
		s.apiURL, instanceID, token, image)
	out, err := runSSH(host, cmd)
	if err != nil {
		log.Printf("[scheduler] ssh docker run on %s failed: %v", host.Addr, err)
		return "", "", err
	}
	containerID = strings.TrimSpace(out)
	return containerID, host.ID, nil
}

// Stop stops and removes a container on the given host.
func (s *Scheduler) Stop(ctx context.Context, hostID, containerID string) error {
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
	cmd := fmt.Sprintf("docker stop %s && docker rm %s", containerID, containerID)
	if _, err := runSSH(host, cmd); err != nil {
		log.Printf("[scheduler] ssh docker stop on %s failed: %v", host.Addr, err)
		return err
	}
	return nil
}
