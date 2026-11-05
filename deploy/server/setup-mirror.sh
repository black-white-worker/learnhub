#!/usr/bin/env bash
# =====================================================================
# Docker 镜像加速（幂等合并 /etc/docker/daemon.json）
#
# 只维护 registry-mirrors 这一个字段：
#   - 文件已存在：保留其它字段（log-driver / live-restore 等），只重排镜像站
#   - 文件不存在：写入完整默认配置（镜像站 + 日志轮转 + live-restore）
#   - 内容没变化：直接退出，不 reload
#   - 有变化：优先 systemctl reload docker（SIGHUP，容器不重启），失败才 restart
#
# 用法：
#   bash deploy/server/setup-mirror.sh
#   bash deploy/server/setup-mirror.sh --aliyun https://xxxx.mirror.aliyuncs.com
#   bash deploy/server/setup-mirror.sh --mirror https://docker.1panel.live
#   bash deploy/server/setup-mirror.sh --dry-run
#   bash deploy/server/setup-mirror.sh --show
#
# 阿里云加速地址在「容器镜像服务 ACR → 镜像工具 → 镜像加速器」里获取，
# 每个账号不同（形如 https://<你的ID>.mirror.aliyuncs.com），不能写死成公共地址。
# 该地址只加速 Docker Hub（docker.io），ghcr.io / quay.io 等不走它。
# =====================================================================
set -euo pipefail

DAEMON_JSON="${DAEMON_JSON:-/etc/docker/daemon.json}"
DEFAULT_MIRRORS=(
  "https://docker.m.daocloud.io"
  "https://docker.1ms.run"
  "https://docker.xuanyuan.me"
)

ALIYUN_URL="${ALIYUN_MIRROR_URL:-}"
EXTRA_MIRRORS=()
DRY_RUN=0
SHOW_ONLY=0

usage() {
  cat <<'EOF'
用法：
  bash setup-mirror.sh                        # 内置三个公共站
  bash setup-mirror.sh --aliyun <url>         # 阿里云专属加速（排在最前）
  bash setup-mirror.sh --mirror <url>         # 追加一个镜像站（可重复）
  bash setup-mirror.sh --dry-run              # 只打印目标配置，不写入
  bash setup-mirror.sh --show                 # 只看当前配置与生效状态
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --aliyun)  ALIYUN_URL="${2:-}"; shift 2 ;;
    --mirror)  EXTRA_MIRRORS+=("${2:-}"); shift 2 ;;
    --dry-run) DRY_RUN=1; shift ;;
    --show)    SHOW_ONLY=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "[x] 未知参数：$1" >&2; exit 2 ;;
  esac
done

if [ "$SHOW_ONLY" = "1" ]; then
  echo "== $DAEMON_JSON =="
  if [ -f "$DAEMON_JSON" ]; then cat "$DAEMON_JSON"; else echo "(不存在)"; fi
  echo
  echo "== docker info（当前生效）=="
  docker info 2>/dev/null | sed -n '/Registry Mirrors/,+6p' || echo "docker 未运行"
  exit 0
fi

[ "$(id -u)" -eq 0 ] || { echo "[x] 需要 root：sudo bash $0" >&2; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "[x] 未安装 docker" >&2; exit 1; }
command -v python3 >/dev/null 2>&1 || { echo "[x] 需要 python3（用于安全合并 JSON）" >&2; exit 1; }

EXTRA_JOINED="$(printf '%s\n' "${EXTRA_MIRRORS[@]:-}")"
DEFAULT_JOINED="$(printf '%s\n' "${DEFAULT_MIRRORS[@]}")"

TMP_JSON="$(mktemp /tmp/daemon.json.XXXXXX)"
trap 'rm -f "$TMP_JSON"' EXIT

DAEMON_JSON="$DAEMON_JSON" ALIYUN_URL="$ALIYUN_URL" \
EXTRA_MIRRORS="$EXTRA_JOINED" DEFAULT_MIRRORS="$DEFAULT_JOINED" \
python3 - "$DAEMON_JSON" > "$TMP_JSON" <<'PY'
import json, os, sys

path = sys.argv[1]
aliyun = os.environ.get("ALIYUN_URL", "").strip()
extra = [x.strip() for x in os.environ.get("EXTRA_MIRRORS", "").splitlines() if x.strip()]
defaults = [x.strip() for x in os.environ.get("DEFAULT_MIRRORS", "").splitlines() if x.strip()]

def norm(u):
    u = u.strip().rstrip("/")
    if not u:
        return ""
    if not u.startswith(("http://", "https://")):
        u = "https://" + u
    return u

old = {}
if os.path.exists(path):
    with open(path, "r", encoding="utf-8") as f:
        raw = f.read().strip()
    if raw:
        try:
            old = json.loads(raw)
        except Exception as e:
            sys.stderr.write("[x] %s 不是合法 JSON，先手工修好再用本脚本：%s\n" % (path, e))
            sys.exit(1)
        if not isinstance(old, dict):
            sys.stderr.write("[x] %s 顶层不是 JSON 对象\n" % path)
            sys.exit(1)

existing = old.get("registry-mirrors") or []
if not isinstance(existing, list):
    existing = []

order = []
for u in [aliyun] + existing + extra + defaults:
    n = norm(u)
    if n and n not in order:
        order.append(n)

cfg = dict(old)
cfg["registry-mirrors"] = order
if not old:
    cfg.setdefault("log-driver", "json-file")
    cfg.setdefault("log-opts", {"max-size": "50m", "max-file": "3"})
    cfg.setdefault("live-restore", True)

sys.stdout.write(json.dumps(cfg, ensure_ascii=False, indent=2) + "\n")
PY

echo "[cfg] 目标配置："
cat "$TMP_JSON"

# 无变化就跳过，避免每次部署都 reload
if [ -f "$DAEMON_JSON" ] && python3 - "$DAEMON_JSON" "$TMP_JSON" <<'PY'
import json, sys
def load(p):
    with open(p, "r", encoding="utf-8") as f:
        return json.load(f)
sys.exit(0 if load(sys.argv[1]) == load(sys.argv[2]) else 1)
PY
then
  echo "[ok] registry-mirrors 无变化，跳过 reload"
  exit 0
fi

if [ "$DRY_RUN" = "1" ]; then
  echo "[dry-run] 未写入（去掉 --dry-run 生效）"
  exit 0
fi

FILE_EXISTED=0
[ -f "$DAEMON_JSON" ] && FILE_EXISTED=1

mkdir -p "$(dirname "$DAEMON_JSON")"
if [ "$FILE_EXISTED" = "1" ]; then
  BACKUP="$DAEMON_JSON.bak.$(date +%Y%m%d-%H%M%S)"
  cp -a "$DAEMON_JSON" "$BACKUP"
  echo "[bak] 已备份：$BACKUP"
fi
install -m 644 "$TMP_JSON" "$DAEMON_JSON"
python3 -m json.tool "$DAEMON_JSON" >/dev/null
echo "[ok] 已写入 $DAEMON_JSON"

if [ "$FILE_EXISTED" = "0" ]; then
  # 首次创建时还写入了 log-driver / log-opts / live-restore，这些只能 restart 生效
  echo "[i] 首次创建 daemon.json，restart 让日志轮转和 live-restore 生效"
  systemctl restart docker
elif systemctl reload docker >/dev/null 2>&1; then
  echo "[ok] docker 已热重载（registry-mirrors 支持 SIGHUP，容器未重启）"
else
  echo "[!] reload 不可用，改用 restart（live-restore 生效时容器不会停）"
  systemctl restart docker
fi

sleep 1
echo
echo "[verify] 当前生效："
docker info 2>/dev/null | sed -n '/Registry Mirrors/,+6p' || true

if ! docker info 2>/dev/null | grep -q "docker.m.daocloud.io"; then
  echo "[!] docker info 里没看到预期的加速地址，reload 可能未生效；执行 sudo systemctl restart docker 再看" >&2
  exit 1
fi
echo "[ok] 镜像加速已生效"
