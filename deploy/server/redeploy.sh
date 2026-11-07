#!/usr/bin/env bash
# =====================================================================
# 服务器端重建脚本（产物模式）
#
# 前提：本机已用 scripts/deploy.ps1 上传过这些产物
#   backend/bin/learningplatform-api        （本机 go build 的 linux/amd64 二进制）
#   frontend/dist                            （本机 npm run build 的静态文件）
#   backend/deploy/sql/migration             （Flyway 迁移 SQL）
#
# 这里只做六件事：
#   0) 闸门：可用内存 / Docker 数据盘不够就直接拒绝部署（基础镜像那步最吃盘）
#   1) 确保运行基础镜像 learningplatform-backend-base:v2 存在（LibreOffice + ffmpeg，只建一次）
#   2) docker compose build lp-backend lp-frontend —— 只 COPY 产物，通常 10~30 秒
#   3) docker image prune -f —— 清掉重建后留下的 <none> 悬空镜像（不删在用的、不删基础镜像）
#   4) 部署前 mysqldump 备份 learningplatform（保留最近 3 份，见下方 BACKUP_DIR）
#   5) 重启容器（含 flyway 迁移）并等健康检查
#
# 用法：
#   bash deploy/server/redeploy.sh            # 闸门 + 构建 + 备份 + 重启 + 等健康
#   bash deploy/server/redeploy.sh build      # 只构建，不备份不重启
#   REBUILD_BASE=1 bash deploy/server/redeploy.sh   # 强制重建基础镜像（改了 apk 包时用）
#
# 备份/闸门相关：
#   SKIP_DB_BACKUP=1   跳过 mysqldump（明知库是空的、或刚还原过时用）
#   BACKUP_DIR         备份目录，默认 /var/backups/learningplatform（刻意放在仓库外）
#   MIN_FREE_MB        重启前要求的可用内存（默认 180；本栈优先级最低，门槛也最低）
#   MIN_FREE_DISK_MB   要求的 Docker 数据盘可用空间（默认 1024；REBUILD_BASE=1 时至少要 3072）
# =====================================================================
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/../.."

ENV_FILE="${ENV_FILE:-.env.prod}"
COMPOSE=(docker compose -f docker-compose.server.yml -f docker-compose.artifact.yml --env-file "$ENV_FILE")
BASE_IMAGE="${BACKEND_RUNTIME_IMAGE:-learningplatform-backend-base:v2}"
MODE="${1:-up}"

die() { echo "[x] $*" >&2; exit 1; }

# 从 env 文件里取一个变量，去掉外层引号；不 source 整个文件，
# 因为 .env.prod 里可能有带 $ 的密码，source 会把它当变量展开。
env_from_file() {
  local key="$1" line val
  line=$(grep -E "^[[:space:]]*${key}=" "$ENV_FILE" | tail -n 1 || true)
  [ -n "$line" ] || return 0
  val="${line#*=}"
  val="${val%$'\r'}"
  case "$val" in
    \"*\") val="${val#\"}"; val="${val%\"}" ;;
    \'*\') val="${val#\'}"; val="${val%\'}" ;;
  esac
  printf '%s' "$val"
}

# 最近一次备份的路径 + 还原命令
latest_backup() {
  local latest
  latest=$(ls -1 "${BACKUP_DIR:-/var/backups/learningplatform}"/learningplatform-*.sql.gz 2>/dev/null | tail -n 1)
  if [ -n "$latest" ]; then
    printf '%s\n    还原：gunzip -c %s | docker exec -i -e MYSQL_PWD=... infra-mysql mysql -uroot %s\n' \
      "$latest" "$latest" "${DB_NAME_BACKUPED:-learningplatform}"
  else
    printf '（本次没有备份，BACKUP_DIR=%s 下没有 dump 文件）\n' "${BACKUP_DIR:-/var/backups/learningplatform}"
  fi
}

[ -f "$ENV_FILE" ] || die "$ENV_FILE 不存在（先 cp .env.prod.example .env.prod 再填）"
[ -f backend/bin/learningplatform-api ] || die "backend/bin/learningplatform-api 不存在，请在本机先跑 scripts/deploy.ps1"
[ -f frontend/dist/index.html ] || die "frontend/dist 不存在，请在本机先跑 scripts/deploy.ps1"
[ -d backend/deploy/sql/migration ] || die "backend/deploy/sql/migration 不存在，Flyway 迁移无法执行"

# ---- 0) 闸门：内存与磁盘 ----
# 磁盘这一项对本栈尤其重要：REBUILD_BASE=1 要 3GB，而 lp 和 videonest 用的是同一个
# Docker 数据盘，盘满会让对面那个项目的 MySQL 一起挂掉。
MIN_FREE_MB="${MIN_FREE_MB:-180}"
MIN_FREE_DISK_MB="${MIN_FREE_DISK_MB:-1024}"
if [ "${REBUILD_BASE:-0}" = "1" ] && [ "$MIN_FREE_DISK_MB" -lt 3072 ]; then
  MIN_FREE_DISK_MB=3072
fi
if [ "$MODE" != "build" ]; then
  AVAIL_MB=$(awk '/^MemAvailable:/{print int($2/1024)}' /proc/meminfo 2>/dev/null || echo "")
  if [ -z "$AVAIL_MB" ]; then
    echo "[!] 读不到 /proc/meminfo，跳过内存闸门（只在这台机器不是 Linux 时才可能）" >&2
  else
    echo "==> 资源闸门：可用内存 ${AVAIL_MB}M（要求 >= ${MIN_FREE_MB}M）"
    if [ "$AVAIL_MB" -lt "$MIN_FREE_MB" ]; then
      echo
      echo "[x] 可用内存只有 ${AVAIL_MB}M。本栈的 LibreOffice 预览一跑就是几百 M，" >&2
      echo "    先把内存留给 videonest 和共享 MySQL：docker stop hp-backend hp-frontend" >&2
      echo "    确认无风险后跳过：MIN_FREE_MB=0 bash $0" >&2
      exit 1
    fi
  fi

  DOCKER_ROOT=$(docker info -f '{{.DockerRootDir}}' 2>/dev/null || echo /var/lib/docker)
  FREE_DISK_MB=$(df -m --output=avail "$DOCKER_ROOT" 2>/dev/null | tail -n 1 | tr -d ' ')
  if [ -n "$FREE_DISK_MB" ]; then
    echo "==> ${DOCKER_ROOT} 可用磁盘 ${FREE_DISK_MB}M（要求 >= ${MIN_FREE_DISK_MB}M）"
    if [ "$FREE_DISK_MB" -lt "$MIN_FREE_DISK_MB" ]; then
      echo
      echo "[x] Docker 数据盘只剩 ${FREE_DISK_MB}M，先清理再部署：" >&2
      echo "      docker image prune -f && docker builder prune -f" >&2
      echo "      docker system df" >&2
      exit 1
    fi
  fi
fi


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

# ---- 4) 部署前备份业务库（保留最近 3 份）----
# 放在 up 之前而不是 build 之前：基础镜像那一跑就是 5~15 分钟，
# 备份失败要中止的话，不该把那十几分钟白花掉。
# up -d 会先跑 flyway 容器做版本化迁移，迁移不可逆，所以这一步和 videonest 一样必须有。
if [ "$MODE" != "build" ]; then
  if [ "${SKIP_DB_BACKUP:-0}" = "1" ]; then
    echo "==> 已按要求跳过部署前数据库备份（SKIP_DB_BACKUP=1）"
  else
    DB_NAME=$(env_from_file DB_NAME)
    DB_NAME="${DB_NAME:-learningplatform}"
    DB_NAME_BACKUPED="$DB_NAME"
    MYSQL_ROOT_PW=$(env_from_file MYSQL_ROOT_PASSWORD)
    BACKUP_DIR="${BACKUP_DIR:-/var/backups/learningplatform}"
    if [ -z "$MYSQL_ROOT_PW" ]; then
      echo "[x] $ENV_FILE 里找不到 MYSQL_ROOT_PASSWORD，无法备份。" >&2
      echo "    确认可以跳过备份：SKIP_DB_BACKUP=1 bash $0" >&2
      exit 1
    elif ! docker ps --format '{{.Names}}' | grep -qx 'infra-mysql'; then
      echo "[!] infra-mysql 未在运行，跳过备份（继续部署，但本次没有可回滚的快照）" >&2
    else
      mkdir -p "$BACKUP_DIR" || die "备份目录 $BACKUP_DIR 建不出来（换 BACKUP_DIR 或先清盘）"
      STAMP=$(date +%Y%m%d-%H%M%S)
      DUMP_FILE="$BACKUP_DIR/learningplatform-${STAMP}.sql.gz"
      echo "==> 备份 ${DB_NAME} -> ${DUMP_FILE}"
      # 密码经 MYSQL_PWD 传入，不落 argv（否则 docker inspect / ps 能看到）
      if docker exec -i -e MYSQL_PWD="$MYSQL_ROOT_PW" infra-mysql \
          mysqldump -uroot --single-transaction --quick --routines --triggers \
                    --databases "$DB_NAME" 2>/tmp/lp-dump.err | gzip > "$DUMP_FILE"; then
        BYTES=$(wc -c < "$DUMP_FILE" | tr -d ' ')
        if [ "${BYTES:-0}" -lt 1024 ]; then
          rm -f "$DUMP_FILE"
          die "备份产物只有 ${BYTES:-0} 字节，判定为失败，不用于回滚"
        fi
        echo "    备份完成：${BYTES} 字节"
        mapfile -t DUMPS < <(ls -1 "$BACKUP_DIR"/learningplatform-*.sql.gz 2>/dev/null || true)
        if [ "${#DUMPS[@]}" -gt 3 ]; then
          for old in "${DUMPS[@]:0:${#DUMPS[@]}-3}"; do
            echo "    清理旧备份 $(basename "$old")"
            rm -f "$old"
          done
        fi
      else
        rm -f "$DUMP_FILE"
        echo "[x] mysqldump 失败：$(tail -n 3 /tmp/lp-dump.err 2>/dev/null)" >&2
        echo "    多半是密码不对或库不存在。修好再部署，或明确跳过：SKIP_DB_BACKUP=1 bash $0" >&2
        exit 1
      fi
    fi
  fi
fi

# ---- 5) 重启 + 健康检查 ----
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
      echo >&2
      echo "    本栈最常见的不健康原因：共享中间件没起（infra-mysql / infra-redis / infra-minio），" >&2
      echo "    以及 flyway 迁移失败导致后端连不上预期表结构。" >&2
      echo "    要退回上一版：$(latest_backup)" >&2
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
echo "==> 可回滚的最近备份：$(latest_backup)"
