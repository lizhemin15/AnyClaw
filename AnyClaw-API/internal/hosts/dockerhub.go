package hosts

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 15 * time.Second}

// getDockerHubToken 获取 Docker Hub 拉取 token（公开镜像无需认证）
func getDockerHubToken(repo string) (string, error) {
	scope := "repository:" + repo + ":pull"
	url := "https://auth.docker.io/token?service=registry.docker.io&scope=" + scope
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var v struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &v); err != nil {
		return "", err
	}
	if v.Token == "" {
		return "", fmt.Errorf("no token in response")
	}
	return v.Token, nil
}

// getDockerHubDigest 获取 Docker Hub 上 :latest 的 manifest digest
func getDockerHubDigest(image string) (string, error) {
	// image 格式: namespace/repo:tag
	parts := strings.SplitN(image, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 && parts[1] != "" {
		tag = parts[1]
	}
	token, err := getDockerHubToken(repo)
	if err != nil {
		return "", err
	}
	url := "https://registry-1.docker.io/v2/" + repo + "/manifests/" + tag
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest in response")
	}
	return digest, nil
}
