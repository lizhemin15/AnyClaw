package hosts

import (
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

// getDockerHubDigestViaHost 通过 SSH 在宿主机上获取 Docker Hub manifest digest
// 必须与 docker inspect RepoDigests 一致（manifest digest），不能用 config digest
func getDockerHubDigestViaHost(runner runCommandOnHost, host *db.Host, image string) (string, error) {
	parts := strings.SplitN(image, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 && parts[1] != "" {
		tag = parts[1]
	}
	// 必须获取 manifest digest（与 RepoDigests 一致），不能用 config digest
	// 多架构镜像：请求 manifest list，取 amd64 的 digest；单架构：请求 v2 manifest 取 Docker-Content-Digest
	cmd := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH
REPO="%s"
TAG="%s"
TOKEN=$(curl -sSL "https://auth.docker.io/token?service=registry.docker.io&scope=repository:${REPO}:pull" 2>/dev/null | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
[ -z "$TOKEN" ] && exit 1
JSON=$(curl -sSL -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.docker.distribution.manifest.list.v2+json" "https://registry-1.docker.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null)
if echo "$JSON" | grep -q '"manifests"'; then
  echo "$JSON" | grep -o '"digest":"sha256:[^"]*"' | head -1 | cut -d'"' -f4
else
  curl -sSL -I -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.docker.distribution.manifest.v2+json" "https://registry-1.docker.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null | grep -i "Docker-Content-Digest" | sed 's/.*: *//' | tr -d '\r\n'
fi`,
		strings.ReplaceAll(repo, `"`, `\"`),
		strings.ReplaceAll(tag, `"`, `\"`))
	out, err := runner.RunCommand(host, cmd)
	if err != nil {
		return "", fmt.Errorf("宿主机请求 Docker Hub 失败: %w", err)
	}
	out = strings.TrimSpace(out)
	if m := sha256Re.FindString(out); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("未获取到 digest: %s", out)
}
