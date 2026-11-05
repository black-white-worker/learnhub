#!/usr/bin/env bash
# 预拉取所有镜像：多个国内镜像站依次尝试，成功后重打成官方标签。
# 可重复执行，已存在的会跳过。
# 用法：bash pull-images.sh
set -uo pipefail

# 按顺序尝试；某个站拉不动会自动换下一个
MIRRORS=(
  "docker.m.daocloud.io"
  "docker.1panel.live"
  "docker.1ms.run"
)
PULL_TIMEOUT=300   # 单个镜像单站最多等 5 分钟

FAILED=()

pull() {
  local src="$1" dst="$2" m
  if docker image inspect "$dst" >/dev/null 2>&1; then
    echo "[skip] $dst"
    return 0
  fi
  for m in "${MIRRORS[@]}"; do
    echo "[pull] $dst  <- $m"
    if timeout "$PULL_TIMEOUT" docker pull "$m/$src" >/tmp/pull-one.log 2>&1; then
      docker tag "$m/$src" "$dst"
      echo "       ok"
      return 0
    fi
    echo "       failed on $m: $(tail -1 /tmp/pull-one.log)"
  done
  echo "       FAILED: $dst" >&2
  FAILED+=("$dst")
}

# ---- 共享中间件 ----
pull "library/mysql:8.4"                         "mysql:8.4"
pull "pgvector/pgvector:pg16"                    "pgvector/pgvector:pg16"
pull "library/redis:7.4-alpine"                  "redis:7.4-alpine"
pull "minio/minio:RELEASE.2025-09-07T16-13-09Z"  "minio/minio:RELEASE.2025-09-07T16-13-09Z"
pull "minio/mc:RELEASE.2025-08-13T08-35-41Z"     "minio/mc:RELEASE.2025-08-13T08-35-41Z"

# ---- 构建基础镜像 ----
# 注：ffmpeg 不再从 mwader/static-ffmpeg 镜像 COPY，改为 Dockerfile 里 apk add ffmpeg
#     （走阿里云源）。少一个 Docker Hub 依赖，国内服务器不会卡在 0B 拉取。
pull "library/golang:1.27-alpine"                     "golang:1.27-alpine"
pull "library/alpine:3.21"                            "alpine:3.21"
pull "library/node:22-alpine"                         "node:22-alpine"
pull "library/nginx:1.27-alpine"                      "nginx:1.27-alpine"
pull "flyway/flyway:10.20.0-alpine"                   "flyway/flyway:10.20.0-alpine"
pull "library/maven:3.9-amazoncorretto-21"            "maven:3.9-amazoncorretto-21"
pull "library/amazoncorretto:21-alpine"               "amazoncorretto:21-alpine"
pull "library/node:20-alpine"                         "node:20-alpine"
pull "library/maven:3.9.11-eclipse-temurin-21-alpine" "maven:3.9.11-eclipse-temurin-21-alpine"
pull "library/eclipse-temurin:21-jre-alpine"          "eclipse-temurin:21-jre-alpine"
pull "library/nginx:1.29-alpine"                      "nginx:1.29-alpine"
pull "library/node:24-alpine"                         "node:24-alpine"
pull "library/rabbitmq:4.2.0-management"              "rabbitmq:4.2.0-management"

echo
echo "================ 结果 ================"
docker images --format '{{.Repository}}:{{.Tag}}' | grep -v -E 'daocloud|1panel|1ms' | sort
if [ ${#FAILED[@]} -gt 0 ]; then
  echo
  echo "以下镜像拉取失败（换网络或稍后重跑本脚本）："
  printf '  - %s\n' "${FAILED[@]}"
  exit 1
fi
echo "全部就绪"
