#!/usr/bin/env bash
# 在服务器上一次性生成 4 份 env（密码随机、四份保持一致）
# 用法：bash init-env.sh
set -euo pipefail

ROOT=/opt/apps
PUB=101.201.208.223

MYSQL_PW=$(openssl rand -hex 16)
REDIS_PW=$(openssl rand -hex 16)
MINIO_PW=$(openssl rand -hex 16)
PG_PW=$(openssl rand -hex 16)
HP_JWT=$(openssl rand -hex 32)
VN_JWT=$(openssl rand -hex 32)
HP_ADMIN=$(openssl rand -hex 8)
RABBIT_PW=$(openssl rand -hex 16)

umask 077

cat > "$ROOT/learningplatform/deploy/server/.env" <<EOF
PUBLIC_HOST=$PUB
MYSQL_ROOT_PASSWORD=$MYSQL_PW
PG_USER=heart_pilot
PG_PASSWORD=$PG_PW
REDIS_PASSWORD=$REDIS_PW
MINIO_ROOT_USER=lpminio
MINIO_ROOT_PASSWORD=$MINIO_PW
EOF

cat > "$ROOT/learningplatform/.env.prod" <<EOF
PUBLIC_HOST=$PUB
HTTP_PORT=80
DB_NAME=learningplatform
MYSQL_ROOT_PASSWORD=$MYSQL_PW
REDIS_PASSWORD=$REDIS_PW
MINIO_ACCESS_KEY=lpminio
MINIO_SECRET_KEY=$MINIO_PW
MINIO_BUCKET=learningplatform
MINIO_PRIVATE=true
EOF

cat > "$ROOT/heart-pilot/.env.prod" <<EOF
PUBLIC_HOST=$PUB
HTTP_PORT=8081
PG_USER=heart_pilot
PG_PASSWORD=$PG_PW
REDIS_PASSWORD=$REDIS_PW
MINIO_ACCESS_KEY=lpminio
MINIO_SECRET_KEY=$MINIO_PW
MINIO_BUCKET=heart-pilot
JWT_SECRET=$HP_JWT
APP_ADMIN_USERNAME=admin
APP_ADMIN_PASSWORD=$HP_ADMIN
CORS_ALLOWED_ORIGINS=http://$PUB:8081
DASHSCOPE_API_KEY=not-configured
SEARCH_API_KEY=
AMAP_MAPS_API_KEY=
PEXELS_API_KEY=
EOF

cat > "$ROOT/videonest/.env.prod" <<EOF
PUBLIC_HOST=$PUB
HTTP_PORT=8082
PUBLIC_SITE_URL=http://$PUB:8082
DB_NAME=videonest
MYSQL_ROOT_PASSWORD=$MYSQL_PW
FLYWAY_BASELINE_VERSION=3
REDIS_PASSWORD=$REDIS_PW
RABBITMQ_USERNAME=videonest
RABBITMQ_PASSWORD=$RABBIT_PW
MINIO_ACCESS_KEY=lpminio
MINIO_SECRET_KEY=$MINIO_PW
MINIO_BUCKET=videonest
JWT_SECRET=$VN_JWT
VIDEO_REVIEW_TIMEOUT_MILLISECONDS=86400000
RESOURCE_RETENTION_DAYS=7
UPLOAD_MAX_VIDEO_SIZE_BYTES=838860800
UPLOAD_MINIMUM_FREE_DISK_BYTES=5368709120
EOF

chmod 600 "$ROOT/learningplatform/deploy/server/.env" \
          "$ROOT/learningplatform/.env.prod" \
          "$ROOT/heart-pilot/.env.prod" \
          "$ROOT/videonest/.env.prod"

echo "env ok"
echo "heart-pilot 管理员密码写在：$ROOT/heart-pilot/.env.prod 的 APP_ADMIN_PASSWORD"
