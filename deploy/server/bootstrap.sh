#!/usr/bin/env bash
# =====================================================================
# 服务器一次性初始化：swap / 内核参数 / Docker / 镜像加速 / 共享网络
# 适用：Alibaba Cloud Linux 3（CentOS 8 系），root 执行
# 用法：bash bootstrap.sh
# =====================================================================
set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "[x] 请用 root 执行：sudo bash bootstrap.sh"
  exit 1
fi

echo "==> 1/6 基础工具"
dnf install -y -q git curl tar vim >/dev/null

echo "==> 2/6 2G swap（2C2G 跑三套应用必须开，否则容易 OOM）"
if [ ! -f /swapfile ]; then
  fallocate -l 2G /swapfile || dd if=/dev/zero of=/swapfile bs=1M count=2048
  chmod 600 /swapfile
  mkswap /swapfile
  swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
  echo "    swap 已创建"
else
  echo "    swap 已存在，跳过"
fi
free -h

echo "==> 3/6 内核参数"
cat >/etc/sysctl.d/99-app.conf <<'EOF'
vm.swappiness = 10
vm.overcommit_memory = 1
net.core.somaxconn = 1024
fs.file-max = 200000
EOF
sysctl --system >/dev/null

echo "==> 4/6 安装 Docker（阿里云镜像源）"
if ! command -v docker >/dev/null 2>&1; then
  dnf install -y -q dnf-plugins-core >/dev/null
  dnf config-manager --add-repo https://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo
  dnf install -y -q docker-ce docker-ce-cli containerd.io docker-compose-plugin >/dev/null
fi
systemctl enable --now docker

echo "==> 5/6 Docker 守护进程配置（镜像加速 + 日志轮转）"
# 统一交给 setup-mirror.sh：幂等合并、自动备份、优先热重载不重启容器。
# 想加阿里云专属加速：先 export ALIYUN_MIRROR_URL=https://xxxx.mirror.aliyuncs.com 再跑本脚本。
BOOTSTRAP_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -n "${ALIYUN_MIRROR_URL:-}" ]; then
  bash "$BOOTSTRAP_DIR/setup-mirror.sh" --aliyun "$ALIYUN_MIRROR_URL"
else
  bash "$BOOTSTRAP_DIR/setup-mirror.sh"
fi

echo "==> 6/6 共享网络"
docker network inspect lp-shared >/dev/null 2>&1 || docker network create lp-shared >/dev/null
echo "    lp-shared 就绪"

echo
echo "==================== 完成 ===================="
docker --version
docker compose version
echo "下一步：填好 4 个 env 文件，再执行 ./deploy.sh all"
