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
	// 官方镜像（无命名空间）需加 library/ 前缀
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	// 使用 HEAD 请求获取 Docker-Content-Digest 响应头，兼容多架构与单架构
	// 显式用 bash 执行，避免 SSH 默认 sh 导致 for/grep -oE 不兼容
	// token 支持 "token" 与 "access_token" 两种字段名
	cmd := fmt.Sprintf(`bash -c 'export PATH=/usr/local/bin:/usr/bin:$PATH
REPO="%s"
TAG="%s"
AUTH_RESP=$(curl -sSL -A "Docker-Client/20.0.0" "https://auth.docker.io/token?service=registry.docker.io&scope=repository:"$REPO":pull" 2>/dev/null)
TOKEN=$(echo "$AUTH_RESP" | grep -oE "\"(token|access_token)\":\"[^\"]*\"" | head -1 | cut -d"\"" -f4)
[ -z "$TOKEN" ] && echo "ERR:token" && exit 1
for ACCEPT in "application/vnd.docker.distribution.manifest.list.v2+json" "application/vnd.docker.distribution.manifest.v2+json" "application/vnd.oci.image.index.v1+json" "application/vnd.oci.image.manifest.v1+json"; do
  DIGEST=$(curl -sSL -I -A "Docker-Client/20.0.0" -H "Authorization: Bearer $TOKEN" -H "Accept: $ACCEPT" "https://registry-1.docker.io/v2/$REPO/manifests/$TAG" 2>/dev/null | grep -i "docker-content-digest" | sed "s/.*: *//" | tr -d "\r\n" | grep -oE "sha256:[a-fA-F0-9]{64}")
  [ -n "$DIGEST" ] && echo "$DIGEST" && exit 0
done
echo ""'`,
		strings.ReplaceAll(repo, `"`, `\"`),
		strings.ReplaceAll(tag, `"`, `\"`))
	out, err := runner.RunCommand(host, cmd)
	if err != nil {
		return "", fmt.Errorf("宿主机请求 Docker Hub 失败: %w", err)
	}
	out = strings.TrimSpace(out)
	if out == "ERR:token" {
		return "", fmt.Errorf("未获取到 Docker Hub token（可能被限流或网络异常）")
	}
	if m := sha256Re.FindString(out); m != "" {
		return m, nil
	}
	return "", fmt.Errorf("未获取到 digest: %s", out)
}
