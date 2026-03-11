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
// 优先使用 docker manifest inspect（宿主机 Docker 自带，与 docker pull 同源），失败时回退到 curl
func getDockerHubDigestViaHost(runner runCommandOnHost, host *db.Host, image string) (string, error) {
	parts := strings.SplitN(image, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 && parts[1] != "" {
		tag = parts[1]
	}
	fullImage := "docker.io/" + repo + ":" + tag

	// 方法1：docker manifest inspect，使用宿主机 Docker 的 registry 客户端（与 pull 同源）
	cmd1 := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH
docker manifest inspect "%s" 2>/dev/null | grep -oE 'sha256:[a-f0-9]{64}' | head -1`,
		strings.ReplaceAll(fullImage, `"`, `\"`))
	if out, err := runner.RunCommand(host, cmd1); err == nil {
		if m := sha256Re.FindString(out); m != "" {
			return m, nil
		}
	}

	// 方法2：curl 回退（部分环境 docker manifest 不可用）
	cmd2 := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH
REPO="%s"
TAG="%s"
TOKEN=$(curl -sSL "https://auth.docker.io/token?service=registry.docker.io&scope=repository:${REPO}:pull" 2>/dev/null | grep -o '"token":"[^"]*"' | cut -d'"' -f4)
[ -z "$TOKEN" ] && exit 1
curl -sSL -I -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.docker.distribution.manifest.v2+json" "https://registry-1.docker.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null | grep -i "Docker-Content-Digest" | sed 's/.*: *//' | tr -d '\r\n'`,
		strings.ReplaceAll(repo, `"`, `\"`),
		strings.ReplaceAll(tag, `"`, `\"`))
	out, err := runner.RunCommand(host, cmd2)
	if err != nil {
		return "", fmt.Errorf("宿主机请求 Docker Hub 失败: %w", err)
	}
	out = strings.TrimSpace(out)
	if m := sha256Re.FindString(out); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("未获取到 digest: %s", out)
}
