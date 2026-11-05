# 三项目同机部署手册（2C2G / Alibaba Cloud Linux 3）

> 目标：一台 `101.201.208.223` 上同时跑 learningplatform、heart-pilot、videonest。
> 做法：**中间件只跑一份**（MySQL / PostgreSQL / Redis / MinIO），三套应用各用自己的 compose，
> 通过 Docker 外部网络 `lp-shared` 访问共享中间件；前端仍是三个独立 nginx，按端口区分。

---

## 0. 先看清楚这台机器的现实

2 vCPU / 2 GiB **同时跑三套**是很紧的：两个 Spring Boot + 一个 Go + MySQL + PG + Redis + MinIO + RabbitMQ。
本方案已经做了裁剪（不部署 ai-service、不部署 MongoDB、JVM 限堆、MySQL 关 performance_schema、开 2G swap），
**能跑起来，但不要指望流畅**。

- 推荐：把实例临时升到 4 GiB（或换 2C4G），配置完全不用改，只是更稳。
- 不升配：严格按本文顺序，构建阶段逐个来，别三个一起 build。
- 只做演示：`./deploy.sh down` 停掉不用的项目，用哪个起哪个。

---

## 1. 端口与地址规划

### 公网（安全组要放行）

| 地址 | 用途 |
|---|---|
| `http://101.201.208.223/` | learningplatform 前端（80） |
| `http://101.201.208.223:8081/` | heart-pilot 前端 |
| `http://101.201.208.223:8082/` | videonest 前端 |
| `http://101.201.208.223:9010` | MinIO S3（预签名上传/播放，必须公网可达） |
| `22/tcp` | SSH |

### 只绑 127.0.0.1（SSH 隧道访问，不要开安全组）

| 端口 | 服务 |
|---|---|
| 3306 | MySQL |
| 5432 | PostgreSQL |
| 6379 | Redis |
| 9011 | MinIO 控制台 |
| 5672 / 15672 | RabbitMQ / 管理台 |
| 8888 | learningplatform 后端 |
| 8123 | heart-pilot 后端 |
| 8080 | videonest 后端 |

隧道示例（本机 PowerShell）：
```powershell
ssh -L 13306:127.0.0.1:3306 -L 19011:127.0.0.1:9011 root@101.201.208.223
```

### 容器内部端口（不映射宿主机）

`lp-backend:8888`、`hp-backend:8123`、`vn-backend:8080`、`mysql:3306`、`postgres:5432`、
`redis:6379`、`minio:9000`、`rabbitmq:5672`。

### 数据隔离方式

- MySQL：两个库 `learningplatform`、`videonest`（同一个 root 账号）
- PostgreSQL：一个库 `heart_pilot`（pgvector 镜像）
- Redis：`db0`=learningplatform，`db1`=videonest，`db2`=heart-pilot
- MinIO：三个桶 `learningplatform`、`heart-pilot`、`videonest`
- RabbitMQ：只有 videonest 用

---

## 2. 阿里云安全组

入方向放行：`22`、`80`、`8081`、`8082`、`9010`。
**不要**放行 3306/6379/5432/5672/15672/9011。

> 如果 `80` 打不开（备案/运营商拦截），把 `learningplatform/.env.prod` 里 `HTTP_PORT` 改成 `8080`，
> 地址变成 `http://101.201.208.223:8080/`，其余不变。

---

## 3. 上传代码

服务器上统一放 `/opt/apps`：

```bash
mkdir -p /opt/apps && cd /opt/apps
```

> **learningplatform 已改为“产物模式”**：本机编译 Go 二进制 + `npm run build`，
> 只把产物和 Dockerfile / compose 上传，服务器全程不编译（见第 6 节）。
> 日常部署直接在本机仓库根目录执行 `.\deploy.cmd` 即可，脚本会自动上传，
> **不需要**先按下面 A/B/C 把整份源码搬到服务器。
> 下面三种方式只在两种情况用：服务器能拉 Git 想保留一份源码副本，
> 或者 heart-pilot / videonest 还没走产物模式时。

三种方式选一种：

**A. 服务器能拉 Git（推荐）**
```bash
git clone <你的仓库地址> learningplatform
git clone <你的仓库地址> heart-pilot
git clone <你的仓库地址> videonest
```

**B. 本机打包上传（Windows PowerShell）**
```powershell
# 在 S:\ 下执行；排除 node_modules / target / .git 会快很多
tar --exclude=node_modules --exclude=target --exclude=.git -czf lp.tar.gz learningplatform
scp lp.tar.gz root@101.201.208.223:/opt/apps/
# heart-pilot、videonest 同理
```
服务器上：
```bash
cd /opt/apps && tar -xzf lp.tar.gz && rm lp.tar.gz
```

**C. 已有旧服务器**：`rsync -av --exclude node_modules --exclude target root@旧IP:/opt/apps/xxx/ /opt/apps/xxx/`

> 注意：本机 `.env`（含真实密码）不要跟着上传；服务器上用 `.env.prod`。

---

## 4. 初始化服务器（只做一次）

```bash
cd /opt/apps/learningplatform/deploy/server
chmod +x bootstrap.sh deploy.sh
sudo bash bootstrap.sh
```

它会做：装 git/curl → 建 2G swap → 调内核参数 → 装 Docker + compose 插件 →
配置镜像加速和日志轮转 → 建 `lp-shared` 网络。

装完确认：
```bash
docker --version && docker compose version && free -h
```

---

## 5. 填四个 env 文件（密码必须前后一致）

```bash
# 1) 共享中间件
cd /opt/apps/learningplatform/deploy/server
cp .env.example .env

# 2) learningplatform
cd /opt/apps/learningplatform
cp .env.prod.example .env.prod

# 3) heart-pilot
cd /opt/apps/heart-pilot
cp .env.prod.example .env.prod

# 4) videonest
cd /opt/apps/videonest
cp .env.prod.example .env.prod
```

然后 `vim` 逐个改，**四份文件里这四项必须完全一致**：

| 变量 | 出现在 |
|---|---|
| `MYSQL_ROOT_PASSWORD` | infra/.env、learningplatform/.env.prod、videonest/.env.prod |
| `REDIS_PASSWORD` | 全部四份 |
| `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | 全部四份 |
| `PG_PASSWORD` | infra/.env、heart-pilot/.env.prod |

生成随机密码：
```bash
openssl rand -base64 24
```

改完收紧权限：
```bash
chmod 600 /opt/apps/learningplatform/deploy/server/.env \
          /opt/apps/learningplatform/.env.prod \
          /opt/apps/heart-pilot/.env.prod \
          /opt/apps/videonest/.env.prod
```

> 首次启动后 MySQL 的 root 密码就写进数据卷了；之后再改 `.env` 里的密码不会自动生效，
> 需要用 `ALTER USER` 改库里的密码，或者删掉 `mysql_data` 卷重来（会丢数据）。

---

## 6. 构建与启动

### 6.1 先起中间件

```bash
cd /opt/apps/learningplatform/deploy/server
./deploy.sh infra      # 拉镜像约 600MB，3Mbps 大概 20~40 分钟，耐心等
```

### 6.2 learningplatform：产物模式（推荐，本机构建）

服务器不编译，**本机**在仓库根目录执行：

```powershell
cd S:\learningplatform
.\deploy.cmd                    # 本机 go build + npm run build → 上传产物 → 服务器只 COPY
.\deploy.cmd -SkipBuild         # 复用 backend\bin\learningplatform-api 与 frontend\dist
.\deploy.cmd -SkipUpload        # 不传产物，用服务器上已有产物重跑
.\deploy.cmd -RebuildBase       # 服务器强制重建基础镜像（改了 Dockerfile.base 时）
```

它做的事：
1. 本机 `go build`（`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`，`-trimpath -ldflags "-s -w"`）→ `backend\bin\learningplatform-api`
2. 本机 `npm ci`（缺 `node_modules` 时）+ `npm run build` → `frontend\dist`
3. 产物 + `Dockerfile.bin` / `Dockerfile.base` / `Dockerfile.dist` / compose / `redeploy.sh` 打成一个约 25MB 的 tar.gz 上传
4. 服务器执行 `deploy/server/redeploy.sh`：确保基础镜像 → COPY 产物建镜像（10~30 秒）→ `up -d`（flyway 先跑迁移）→ 等健康 → `docker image prune -f` 清理悬空镜像

首次部署时，`redeploy.sh` 会先构建运行基础镜像 `learningplatform-backend-base:v2`
（LibreOffice + 中文字体 + ffmpeg，约 5~15 分钟、取决于带宽，**只做一次**）；之后每次部署只 COPY 二进制和 dist。

服务器上直接重跑（不用重新上传）：

```bash
cd /opt/apps/learningplatform
bash deploy/server/redeploy.sh          # 构建 + 重启 + 等健康
bash deploy/server/redeploy.sh build    # 只构建，不重启
REBUILD_BASE=1 bash deploy/server/redeploy.sh   # 强制重建基础镜像
```

### 6.3 heart-pilot / videonest

```bash
./deploy.sh app hp
./deploy.sh app vn
```

```powershell
cd S:\heart-pilot; .\deploy.cmd
cd S:\videonest;   .\deploy.cmd
```

### 6.4 还走源码模式（回退 / 首次拉源码）

`./deploy.sh app lp` 仍然可用：它只用 `docker-compose.server.yml`，
在服务器上按 `backend/Dockerfile`、`frontend/Dockerfile` 从源码编译，
适合想完整验证一遍构建链路的场景，但 2C2G 上要 8~15 分钟。

时间参考（2C2G + 3Mbps）：learningplatform 产物模式 10~30 秒（首次含基础镜像 5~15 分钟），
源码模式 8~15 分钟；heart-pilot 10~20 分钟（Maven 下载依赖），videonest 10~20 分钟（Maven + ffmpeg + npm）。

> Windows 入口（`deploy.cmd`）不会动服务器上的 `.env.prod`，也不会动中间件和数据卷。

看状态：
```bash
./deploy.sh status
```

---

## 7. 验证清单

```bash
# 中间件
docker exec infra-mysql mysqladmin -uroot -p"$MYSQL_ROOT_PASSWORD" ping
docker exec infra-redis redis-cli -a "$REDIS_PASSWORD" ping

# 应用（都在服务器上执行）
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1/                 # 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8081/            # 200
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:8082/            # 200
curl -s http://127.0.0.1:8888/healthz
curl -s http://127.0.0.1:8123/api/health
curl -s 'http://127.0.0.1:8080/api/videos/hot?limit=1'
```

浏览器逐个打开：
- `http://101.201.208.223/` → 学习平台，用 Flyway 预置账号登录
- `http://101.201.208.223:8081/` → heart-pilot，用 `.env.prod` 里的 `APP_ADMIN_USERNAME/PASSWORD`
- `http://101.201.208.223:8082/` → videonest，先注册再登录

---

## 8. 日常运维

```bash
cd /opt/apps/learningplatform/deploy/server

./deploy.sh status          # 容器 + 内存 + 磁盘
./deploy.sh logs lp         # 跟某个项目日志
docker logs -f infra-mysql  # 中间件日志
./deploy.sh stop            # 停三个应用，保留中间件和数据
./deploy.sh down            # 全停（数据卷保留）
```

更新某个项目：
```bash
# heart-pilot / videonest：服务器拉代码后重建
cd /opt/apps/videonest && git pull
cd /opt/apps/learningplatform/deploy/server && ./deploy.sh app vn

# learningplatform：在本机仓库根目录跑 .\deploy.cmd（产物模式，见 6.2）
# 或在服务器上直接重跑已有产物：
cd /opt/apps/learningplatform && bash deploy/server/redeploy.sh
```

磁盘清理（镜像越积越多时）：
```bash
docker image prune -f
docker builder prune -f --filter until=168h
```
> `deploy/server/redeploy.sh` 和 `deploy.sh build/app/up` 会在构建/启动完成后自动执行
> `docker image prune -f`（只删 `<none>` 悬空镜像，不动在用的镜像和基础镜像），
> 一般不需要手工清理；上面的命令用于手动补一次。

备份：
```bash
mkdir -p /opt/backup
docker exec infra-mysql sh -c 'exec mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --databases learningplatform videonest' \
  | gzip > /opt/backup/mysql-$(date +%F).sql.gz
docker exec infra-postgres pg_dump -U heart_pilot heart_pilot | gzip > /opt/backup/pg-$(date +%F).sql.gz
docker run --rm -v lp-infra_minio_data:/data -v /opt/backup:/backup alpine \
  tar czf /backup/minio-$(date +%F).tar.gz -C /data .
```

---

## 9. 故障排查

**容器起来就退出 / 被 OOM 杀**
```bash
dmesg | grep -i 'killed process'
docker inspect <容器名> --format '{{.State.OOMKilled}}'
```
处理：确认 swap 生效（`free -h` 有 2G swap），停掉暂时不用的项目，或升配到 4G。

**镜像拉不下来 / 一直 0B 不动**
`/etc/docker/daemon.json` 里的镜像站可能失效。直接重跑配置脚本换一批（幂等、自动备份、优先热重载不重启容器）：

```bash
bash /opt/apps/learningplatform/deploy/server/setup-mirror.sh --show      # 看当前配置与生效状态
bash /opt/apps/learningplatform/deploy/server/setup-mirror.sh --mirror https://docker.1panel.live
```

也可以在本机 `docker save` 后 `scp` 上服务器 `docker load`。

> learningplatform 的后端基础镜像**不再依赖 `mwader/static-ffmpeg`**（那个镜像要走 Docker Hub
> 拉 76MB，国内经常卡在 0B）。ffmpeg 现在和 videonest 一样用 `apk add ffmpeg`（阿里云源），
> 只有 `alpine:3.21`、`nginx:1.27-alpine` 这些基础镜像还需要拉；
> `redeploy.sh` 发现本地缺失会自动按国内镜像站顺序预拉，单站最多等 5 分钟。
> 还是慢的话先跑一次 `bash deploy/server/pull-images.sh`。

**前端 502**
后端没起来或没 healthy：`docker logs lp-backend --tail=100`。
learningplatform 常见是 MySQL 连不上（密码不一致），videonest 常见是 RabbitMQ 未就绪。

**基础镜像卡在 `Installing xxx` 不动**
`Installing` 这行表示包已经下载完、正在解包。2C2G 上解包 LibreOffice / 字体会吃满 CPU 几分钟，
先另开一个 SSH 窗口确认：

```bash
top -b -n1 | head -15     # apk / fc-cache 占着 CPU 就是在干活
free -h                   # 内存 / swap 是否已经打满
df -h /                   # 磁盘是否满了
```

`apk` 或 `fc-cache` 在跑就再等几分钟；CPU 很闲且没有 apk 进程，就 `Ctrl+C` 重跑。
基础镜像现在拆成三层（工具/ffmpeg、LibreOffice、字体），字体也换成更小的
`font-wqy-zenhei`，重跑只重做没完成的那一层，apk 包缓存会复用。
旧版本留下的 `learningplatform-backend-base:latest` 可以 `docker rmi` 掉（新脚本用 `:v2`）。

**基础镜像最后报 `1 error; 922 MiB in 301 packages`**
这不是下载失败：301 个包已经装完了，失败的是收尾的 apk post-install trigger
（日志里 `Executing xxxx.trigger` 那几个，2C2G 上 `gtk-update-icon-cache` /
`gdk-pixbuf` / `fontconfig` 这类 trigger 很容易挂）。现已改成：

- LibreOffice / 字体两层用 `apk add --no-scripts` 跳过 triggers
- 装完后手动补真正影响运行的缓存：`gdk-pixbuf-query-loaders --update-cache`、
  `update-mime-database`、`glib-compile-schemas`、`gtk-update-icon-cache`、`fc-cache -f`
  （都带 `|| true`，个别工具缺失也不影响镜像构建）

所以正常不会再以这个方式失败。如果还有问题，先把 `df -h /`（至少留 3GB）、
`free -h`、`dmesg | grep -i killed` 的结果看一下；只清构建缓存不动镜像：
`docker builder prune -f`。

**上传成功但图片/视频 404**
`MINIO_PUBLIC_ENDPOINT` 必须是 `公网IP:9010`（learningplatform 不带 `http://`，videonest 带）。
安全组没放行 9010 也会 404/超时。

**heart-pilot 页面跨域**
`CORS_ALLOWED_ORIGINS` 必须写成 `http://101.201.208.223:8081`（带端口、不带斜杠）。

**videonest 上传报磁盘不足**
默认要求 10GB 空闲，小盘会误判。`.env.prod` 里已把 `UPLOAD_MINIMUM_FREE_DISK_BYTES` 调到 5GB，
还不够就继续调小。

**时间不对**
所有 compose 都注入了 `TZ=Asia/Shanghai`，宿主机也建议 `timedatectl set-timezone Asia/Shanghai`。

**80 端口打不开**
先用 `curl http://127.0.0.1/` 确认容器正常，再查安全组；仍不行就把 `HTTP_PORT` 改 8080。

---

## 10. 国内镜像加速（已内置）

| 链路 | 使用的源 | 生效位置 |
|---|---|---|
| Docker daemon 兜底 | 阿里云专属加速（可选）+ `docker.m.daocloud.io` / `docker.1ms.run` / `docker.xuanyuan.me` | `/etc/docker/daemon.json`，由 `deploy/server/setup-mirror.sh` 幂等维护 |
| Docker 基础镜像 | `docker.m.daocloud.io` 拉取后重打标签 | `deploy/server/pull-images.sh` |
| Maven 依赖 | `https://maven.aliyun.com/repository/public` | heart-pilot / videonest 的 `settings.xml` |
| npm 依赖 | `https://registry.npmmirror.com` | 三个前端的 `Dockerfile`，以及 `scripts/deploy.ps1` 的 `npm ci` |
| Alpine apk | `mirrors.aliyun.com` | learningplatform / videonest 后端 `Dockerfile` |
| Go 模块 | `goproxy.cn` | learningplatform 后端 `Dockerfile`（原有） |

### 10.1 Docker 镜像加速脚本

`setup-mirror.sh` 只维护 `registry-mirrors` 一个字段：已存在的 `log-driver` / `live-restore` 等原样保留，改动先备份再写，能用 SIGHUP 热重载就不重启 Docker（正在跑的 11 个容器不受影响）。

```bash
# 看当前状态
bash deploy/server/setup-mirror.sh --show

# 用内置三个公共站（默认）
bash deploy/server/setup-mirror.sh

# 加阿里云专属加速（地址在 ACR 控制台「镜像加速器」里取，每个账号不同）
bash deploy/server/setup-mirror.sh --aliyun https://xxxx.mirror.aliyuncs.com

# 只想预览将写入什么，不落盘
bash deploy/server/setup-mirror.sh --aliyun https://xxxx.mirror.aliyuncs.com --dry-run
```

本机也可以用部署脚本一步完成：

```powershell
.\deploy.cmd -MirrorOnly -AliyunMirror https://xxxx.mirror.aliyuncs.com   # 只配镜像加速
.\deploy.cmd -SetupMirror -AliyunMirror https://xxxx.mirror.aliyuncs.com # 配完再正常部署
```

注意：`registry-mirrors` 只对 Docker Hub（`docker.io`）生效，`ghcr.io` / `quay.io` / 各类 `registry.cn-*.aliyuncs.com` 走各自的地址，不受影响。

仍然慢或失败时：

- **拉镜像失败**：重跑 `bash pull-images.sh`，已存在的会 `[skip]`，脚本最后会列出失败项。
- **Maven 卡住**：重跑构建即可，`/root/.m2/repository` 已挂 BuildKit cache，第二次不会重下。
- **npm 卡住**：同上，`/root/.npm` 已挂 cache。
- **apk 失败**：确认 `dl-cdn.alpinelinux.org` 已被替换成 `mirrors.aliyun.com`（构建日志里能看到）。

> `pull-images.sh` 必须在 `deploy.sh app ...` 之前跑过，否则 `docker compose build` 会去 Docker Hub 拉基础镜像并超时。

## 11. 迁移本地已有数据（可选）

新库是空的。想把本机数据带过去：

```powershell
# 本机导出（以 videonest 的 3307 为例）
docker exec videonest-mysql mysqldump -uroot -pJava666 --databases videonest > vn.sql
scp vn.sql root@101.201.208.223:/opt/backup/
```
```bash
# 服务器导入（先起 infra，再导入，最后启动应用让 Flyway 校验）
docker exec -i infra-mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" < /opt/backup/vn.sql
```
heart-pilot 用 `pg_dump` / `pg_restore` 同理；MinIO 数据用 `mc mirror` 或打包卷目录。

> 导入旧库后如果 Flyway 报校验失败，说明旧库的迁移历史和新代码不一致，
> 需要按各项目 `deploy/README` 的说明处理，不要直接 `flyway repair`。
