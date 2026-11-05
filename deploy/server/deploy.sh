#!/usr/bin/env bash
# =====================================================================
# 一键部署脚本
# 目录约定（默认 /opt/apps）：
#   /opt/apps/learningplatform   ← 本仓库（deploy/server 在这里）
#   /opt/apps/heart-pilot
#   /opt/apps/videonest
# 用法：
#   ./deploy.sh infra          起共享中间件并等健康
#   ./deploy.sh build lp|hp|vn 只构建镜像（建议逐个构建，避免内存峰值叠加）
#   ./deploy.sh up    lp|hp|vn 启动单个应用
#   ./deploy.sh app   lp|hp|vn 构建 + 启动
#   ./deploy.sh all             infra + 三个应用依次构建启动
#   ./deploy.sh status          看容器状态和健康
#   ./deploy.sh logs  lp|hp|vn  跟日志
#   ./deploy.sh stop            停三个应用（保留数据）
#   ./deploy.sh down            停三个应用 + 中间件
# =====================================================================
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APPS_ROOT="${APPS_ROOT:-$(cd "$HERE/../../.." && pwd)}"
INFRA_DIR="$HERE"
LP_DIR="$APPS_ROOT/learningplatform"
HP_DIR="$APPS_ROOT/heart-pilot"
VN_DIR="$APPS_ROOT/videonest"

die() { echo "[x] $*" >&2; exit 1; }

need_env() {
  [ -f "$1" ] || die "缺少 $1（先 cp 对应的 .env*.example 再填）"
}

app_dir() {
  case "$1" in
    lp) echo "$LP_DIR" ;;
    hp) echo "$HP_DIR" ;;
    vn) echo "$VN_DIR" ;;
    *) die "未知应用：$1（可选 lp/hp/vn）" ;;
  esac
}

wait_healthy() {
  local name="$1" i status
  for i in $(seq 1 60); do
    status=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$name" 2>/dev/null || echo missing)
    [ "$status" = "healthy" ] && { echo "    $name healthy"; return 0; }
    sleep 3
  done
  echo "[!] $name 60 次探测仍未 healthy，继续执行（看 deploy.sh status / logs）"
  return 0
}

do_infra() {
  need_env "$INFRA_DIR/.env"
  echo "==> 启动共享中间件"
  docker compose -f "$INFRA_DIR/docker-compose.infra.yml" --env-file "$INFRA_DIR/.env" up -d
  wait_healthy infra-mysql
  wait_healthy infra-postgres
  wait_healthy infra-redis
  wait_healthy infra-minio
}

do_build() {
  local dir; dir="$(app_dir "$1")"
  need_env "$dir/.env.prod"
  echo "==> 构建 $1（$dir）"
  docker compose -f "$dir/docker-compose.server.yml" --env-file "$dir/.env.prod" build
}

do_prune() {
  # 只删 <none>:<none> 悬空镜像；仍被任何容器引用的镜像不会被删
  echo "==> 清理悬空镜像"
  docker image prune -f || true
}

do_up() {
  local dir; dir="$(app_dir "$1")"
  need_env "$dir/.env.prod"
  echo "==> 启动 $1"
  docker compose -f "$dir/docker-compose.server.yml" --env-file "$dir/.env.prod" up -d
  # 容器切换完成后，上一个版本的镜像才真正无引用
  do_prune
}

cmd="${1:-help}"
case "$cmd" in
  infra) do_infra ;;
  build) [ $# -ge 2 ] || die "用法：./deploy.sh build lp|hp|vn"; do_build "$2"; do_prune ;;
  up)    [ $# -ge 2 ] || die "用法：./deploy.sh up lp|hp|vn"; do_up "$2" ;;
  app)   [ $# -ge 2 ] || die "用法：./deploy.sh app lp|hp|vn"; do_build "$2"; do_up "$2" ;;
  all)
    do_infra
    for a in lp hp vn; do do_build "$a"; do_up "$a"; done
    echo "==> 全部完成，执行 ./deploy.sh status 检查"
    ;;
  stop)
    for a in lp hp vn; do
      d="$(app_dir "$a")"
      docker compose -f "$d/docker-compose.server.yml" --env-file "$d/.env.prod" stop || true
    done
    ;;
  down)
    for a in vn hp lp; do
      d="$(app_dir "$a")"
      docker compose -f "$d/docker-compose.server.yml" --env-file "$d/.env.prod" down || true
    done
    docker compose -f "$INFRA_DIR/docker-compose.infra.yml" --env-file "$INFRA_DIR/.env" down
    ;;
  status)
    docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
    echo
    free -h
    echo
    df -h / | tail -1
    ;;
  logs)
    [ $# -ge 2 ] || die "用法：./deploy.sh logs lp|hp|vn"
    d="$(app_dir "$2")"
    docker compose -f "$d/docker-compose.server.yml" --env-file "$d/.env.prod" logs -f --tail=200
    ;;
  *)
    sed -n '3,20p' "$0"
    ;;
esac
