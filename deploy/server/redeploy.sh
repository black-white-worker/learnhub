#!/usr/bin/env bash
# =====================================================================
# 服务器端重建脚本（产物模式）
#
# 前提：本机已用 scripts/deploy.ps1 上传过这些产物
#   backend/bin/learningplatform-api        （本机 go build 的 linux/amd64 二进制）
#   frontend/dist                            （本机 npm run build 的静态文件）
#   backend/deploy/sql/migration             （Flyway 迁移 SQL）
#
# 这里只做四件事：
#   1) 确保运行基础镜像 learningplatform-backend-base:v2 存在（LibreOffice + ffmpeg，只建一次）
#   2) docker compose build lp-backend lp-frontend —— 只 COPY 产物，通常 10~30 秒
#   3) 重启容器（含 flyway 迁移）并等健康检查
#   4) docker image prune -f —— 清掉重建后留下的 <none> 悬空镜像（不删在用的、不删基础镜像）
#
# 用法：
#   bash deploy/server/redeploy.sh            # 构建 + 重启 + 等健康
#   bash deploy/server/redeploy.sh build      # 只构建，不重启
#   REBUILD_BASE=1 bash deploy/server/redeploy.sh   # 强制重建基础镜像（改了 apk 包时用）
# =====================================================================
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

ENV_FILE="${ENV_FILE:-.env.prod}"
COMPOSE=(docker compose -f docker-compose.server.yml -f docker-compose.artifact.yml --env-file "$ENV_FILE")
BASE_IMAGE="${BACKEND_RUNTIME_IMAGE:-learningplatform-backend-base:v2}"
MODE="${1:-up}"

die() { echo "[x] $*" >&2; exit 1; }

[ -f "$ENV_FILE" ] || die "$ENV_FILE 不存在（先 cp .env.prod.example .env.prod 再填）"
[ -f backend/bin/learningplatform-api ] || die "backend/bin/learningplatform-api 不存在，请在本机先跑 scripts/deploy.ps1"
[ -f frontend/dist/index.html ] || die "frontend/dist 不存在，请在本机先跑 scripts/deploy.ps1"
[ -d backend/deploy/sql/migration ] || die "backend/deploy/sql/migration 不存在，Flyway 迁移无法执行"

# ---- 1) 运行基础镜像（LibreOffice + 字体 + ffmpeg），只建一次 ----

# alpine 不在本地时，按国内镜像站顺序预拉，避免 docker build 直接连 Docker Hub 卡在 0B
ensure_image() {
  local dst="$1" src="$2" m
  if docker image inspect "$dst" >/dev/null 2>&1; then
    return 0
  fi
  echo "[pull] $dst 不在本地，尝试国内镜像站预拉（最多 2 分钟/站）"
  for m in docker.m.daocloud.io docker.1panel.live docker.1ms.run; do
    echo "       <- $m/$src"
    if timeout 120 docker pull "$m/$src" >/tmp/lp-pull.log 2>&1; then
      docker tag "$m/$src" "$dst"
      echo "       ok"
      return 0
    fi
    echo "       failed: $(tail -1 /tmp/lp-pull.log)"
  done
  echo "[!] $dst 预拉失败，后续 build/up 仍会再试一次（可能很慢）"
  return 1
}

build_base_image() {
  echo "[base] 当前磁盘（构建期间需要约 3GB 可用）："
  df -h / | tail -1
  if docker build -f backend/Dockerfile.base -t "$BASE_IMAGE" backend; then
    return 0
  fi
  echo "" >&2
  echo "[!] 基础镜像构建失败。排查信息：" >&2
  df -h / >&2 || true
  free -h >&2 || true
  docker system df >&2 || true
  echo "[!] 常见原因：磁盘满 / 内存不足（某个 apk trigger 被 OOM）/ 某个 apk trigger 报错。" >&2
  echo "    看日志里最后一行 Executing xxx.trigger 后面紧跟的错误；清缓存后重跑：" >&2
  echo "      docker builder prune -f   # 只清构建缓存，不动在用的镜像和容器" >&2
  exit 1
}

if [ "${REBUILD_BASE:-0}" = "1" ]; then
  echo "[base] REBUILD_BASE=1，重建 $BASE_IMAGE（5~15 分钟，取决于带宽）"
  ensure_image "alpine:3.21" "library/alpine:3.21" || true
  build_base_image
elif docker image inspect "$BASE_IMAGE" >/dev/null 2>&1; then
  echo "[base] $BASE_IMAGE 已存在，跳过（要重建：REBUILD_BASE=1 或 docker rmi $BASE_IMAGE）"
else
  echo "[base] 首次构建 $BASE_IMAGE（下载 LibreOffice + 中文字体 + ffmpeg，约 5~15 分钟，只做一次）"
  echo "       构建日志里能看到 apk 下载进度（走阿里云源，不再拉 Docker Hub 的 ffmpeg 镜像）"
  ensure_image "alpine:3.21" "library/alpine:3.21" || true
  build_base_image
fi

# 前端 Dockerfile.dist 的基础镜像：不在本地时同样先走镜像站
ensure_image "nginx:1.27-alpine" "library/nginx:1.27-alpine" || true

# 让 compose build 的 --build-arg 和上面实际构建/检查的镜像 tag 保持一致
export BACKEND_RUNTIME_IMAGE="$BASE_IMAGE"

# ---- 2) 只重建两个应用镜像（仅 COPY 产物）----
echo "==> 构建 lp-backend / lp-frontend（仅 COPY 产物）"
BUILD_START=$(date +%s)
"${COMPOSE[@]}" build lp-backend lp-frontend
BUILD_SECONDS=$(( $(date +%s) - BUILD_START ))
echo "==> 构建完成，用时 ${BUILD_SECONDS}s"

# ---- 3) 悬空镜像清理（只构建模式也会执行）----
prune_images() {
  # 只删 <none>:<none> 悬空镜像；仍被任何容器引用的镜像不会被删
  echo "==> 清理悬空镜像（docker image prune -f）"
  docker image prune -f || true
}

if [ "$MODE" = "build" ]; then
  prune_images
  echo "==> 只构建模式，未重启容器"
  exit 0
fi

# ---- 4) 重启 + 健康检查 ----
echo "==> 重建容器（flyway 会先跑迁移）"
# flyway 镜像不在本地时也先走镜像站，避免 up 阶段卡在 Docker Hub
ensure_image "flyway/flyway:10.20.0-alpine" "flyway/flyway:10.20.0-alpine" || true
UP_START=$(date +%s)
"${COMPOSE[@]}" up -d
echo "==> 容器已重建，用时 $(( $(date +%s) - UP_START ))s"

echo "==> 等待 lp-backend healthy"
HEALTH="unknown"
for _ in $(seq 1 40); do
  HEALTH=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' lp-backend 2>/dev/null || echo missing)
  case "$HEALTH" in
    healthy) echo "    lp-backend healthy"; break ;;
    unhealthy)
      echo "[!] lp-backend unhealthy，最近日志：" >&2
      docker logs --tail 80 lp-backend >&2 || true
      exit 1
      ;;
  esac
  sleep 3
done
[ "$HEALTH" = "healthy" ] || echo "[!] 等了 120 秒仍未 healthy（$HEALTH），请查看 docker logs lp-backend"

RUNNING=$(docker inspect -f '{{.State.Running}}' lp-frontend 2>/dev/null || echo false)
[ "$RUNNING" = "true" ] || { echo "[!] lp-frontend 未运行" >&2; docker logs --tail 40 lp-frontend >&2 || true; exit 1; }
echo "    lp-frontend running"

# 容器已切到新镜像，上一个版本的镜像这时才真正无引用，可以清掉
prune_images

echo
"${COMPOSE[@]}" ps
echo
echo "==> 完成：镜像构建 ${BUILD_SECONDS}s，本次部署总耗时约 $(( $(date +%s) - BUILD_START ))s"
