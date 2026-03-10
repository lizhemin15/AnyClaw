#!/bin/bash
# AnyClaw 更新脚本：拉取最新镜像、删除旧容器、启动新容器、清理旧镜像
# 用法：chmod +x update-anyclaw.sh && ./update-anyclaw.sh

set -e

# ========== 配置（按需修改） ==========
DOCKER_USER="jamlily"
IMAGE="${DOCKER_USER}/anyclaw-manager:latest"
CONTAINER_NAME="anyclaw-manager"

# MySQL + 域名（按需取消注释）
# export ANYCLAW_NETWORK="anyclaw-net"
# export ANYCLAW_DB_DSN="root:密码@tcp(anyclaw-mysql:3306)/anyclaw?parseTime=true&charset=utf8mb4"
# export ANYCLAW_API_URL="https://open-claw.click"

# ========== 执行更新 ==========
echo ">>> 拉取最新镜像: $IMAGE"
docker pull "$IMAGE"

echo ">>> 停止并删除旧容器: $CONTAINER_NAME"
docker stop "$CONTAINER_NAME" 2>/dev/null || true
docker rm "$CONTAINER_NAME" 2>/dev/null || true

echo ">>> 启动新容器"
RUN_OPTS=(
  -d
  --name "$CONTAINER_NAME"
  -p 8080:8080
  -v anyclaw-data:/data
  --restart unless-stopped
)

# 若配置了网络（连接 MySQL）
if [ -n "${ANYCLAW_NETWORK:-}" ]; then
  RUN_OPTS+=(--network "$ANYCLAW_NETWORK")
fi

# 若配置了 DB 和 API URL
if [ -n "${ANYCLAW_DB_DSN:-}" ]; then
  RUN_OPTS+=(-e "ANYCLAW_DB_DSN=$ANYCLAW_DB_DSN")
fi
if [ -n "${ANYCLAW_API_URL:-}" ]; then
  RUN_OPTS+=(-e "ANYCLAW_API_URL=$ANYCLAW_API_URL")
fi

docker run "${RUN_OPTS[@]}" "$IMAGE"

echo ">>> 清理悬空镜像"
docker image prune -f

echo ">>> 完成"
docker ps --filter "name=$CONTAINER_NAME"
