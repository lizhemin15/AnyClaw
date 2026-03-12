package hosts

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/anyclaw/anyclaw-api/internal/db"
)

var sha256Re = regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)

// runCommandOnHost 在宿主机上执行命令的接口
type runCommandOnHost interface {
	RunCommand(host *db.Host, cmd string) (string, error)
}

// manifestListManifest 用于解析 manifest list 中的单个 manifest
type manifestListManifest struct {
	Digest string `json:"digest"`
}

// manifestListV2 用于解析 Docker manifest list v2
type manifestListV2 struct {
	Manifests []manifestListManifest `json:"manifests"`
}

// getDockerHubDigestsViaHost 通过 SSH 在宿主机上获取 Docker Hub 的所有 digest（manifest list 中各平台 + list 本身）
// 本地 RepoDigests 存的是平台 digest，而 HEAD 返回的可能是 list digest，需解析 list 获取全部 digest 再比较
func getDockerHubDigestsViaHost(runner runCommandOnHost, host *db.Host, image string) ([]string, error) {
	parts := strings.SplitN(image, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 && parts[1] != "" {
		tag = parts[1]
	}
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	// GET manifest，-i 包含响应头（Docker-Content-Digest）和 body；解析 list 获取各平台 digest
	cmd := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH
REPO="%s"
TAG="%s"
TOKEN=$(curl -sSL -A "Docker-Client/20.0.0" "https://auth.docker.io/token?service=registry.docker.io&scope=repository:${REPO}:pull" 2>/dev/null | grep -oE '"(token|access_token)":"[^"]*"' | head -1 | cut -d'"' -f4)
[ -z "$TOKEN" ] && echo "ERR:token" && exit 1
curl -sSL -i -A "Docker-Client/20.0.0" -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json" "https://registry-1.docker.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null`,
		strings.ReplaceAll(repo, `"`, `\"`),
		strings.ReplaceAll(tag, `"`, `\"`))
	out, err := runner.RunCommand(host, cmd)
	if err != nil {
		return nil, fmt.Errorf("宿主机请求 Docker Hub 失败: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "ERR:token" {
		return nil, fmt.Errorf("未获取到 Docker Hub token（可能被限流或网络异常）")
	}
	digests := make(map[string]bool)
	// 先收集所有 sha256 digest（含响应头 Docker-Content-Digest 与 body 中的 digest）
	for _, m := range sha256Re.FindAllString(out, -1) {
		digests[m] = true
	}
	// 解析 manifest list body（-i 输出为 headers + 空行 + body，取空行后的 JSON）
	if idx := strings.Index(out, "\n\n"); idx >= 0 {
		body := strings.TrimSpace(out[idx+2:])
		var list manifestListV2
		if json.Unmarshal([]byte(body), &list) == nil && len(list.Manifests) > 0 {
			for _, m := range list.Manifests {
				if m.Digest != "" {
					digests[m.Digest] = true
				}
			}
		}
	}
	if len(digests) == 0 {
		return nil, fmt.Errorf("未获取到 digest")
	}
	result := make([]string, 0, len(digests))
	for d := range digests {
		result = append(result, d)
	}
	return result, nil
}

// getDockerHubDigestViaHost 兼容旧调用，返回第一个 digest
func getDockerHubDigestViaHost(runner runCommandOnHost, host *db.Host, image string) (string, error) {
	digests, err := getDockerHubDigestsViaHost(runner, host, image)
	if err != nil {
		return "", err
	}
	if len(digests) == 0 {
		return "", fmt.Errorf("未获取到 digest")
	}
	return digests[0], nil
}
