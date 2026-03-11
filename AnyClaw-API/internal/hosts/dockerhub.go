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

// getDockerHubDigestViaHost 通过 SSH 在宿主机上请求 Docker Hub，获取 manifest digest
// 使用宿主机网络，确保宿主机可访问 Docker Hub 时能正常获取
func getDockerHubDigestViaHost(runner runCommandOnHost, host *db.Host, image string) (string, error) {
	parts := strings.SplitN(image, ":", 2)
	repo := parts[0]
	tag := "latest"
	if len(parts) > 1 && parts[1] != "" {
		tag = parts[1]
	}
	// 在宿主机上执行：curl 获取 token 和 manifest，宿主机网络可访问 Docker Hub
	cmd := fmt.Sprintf(`export PATH=/usr/local/bin:/usr/bin:$PATH
REPO="%s"
TAG="%s"
TOKEN=$(curl -sS "https://auth.docker.io/token?service=registry.docker.io&scope=repository:${REPO}:pull" 2>/dev/null | sed -n 's/.*"token":"\\([^"]*\\)".*/\1/p')
[ -z "$TOKEN" ] && exit 1
curl -sS -I -H "Authorization: Bearer $TOKEN" -H "Accept: application/vnd.docker.distribution.manifest.v2+json" "https://registry-1.docker.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null | sed -n 's/.*[Dd]ocker-[Cc]ontent-[Dd]igest: *//p' | tr -d '\r\n'`,
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
