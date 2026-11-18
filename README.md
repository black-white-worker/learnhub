# 学习平台（Learning Platform）

> **当前状态：本地开发阶段，尚未部署到公网服务器。**
> 文中地址均为本机（localhost）调试地址；`deploy/server/`、`docker-compose.server.yml`
> 是已经准备好的服务器部署方案，但项目还没有上线，域名、HTTPS、公网访问都未接入。

面向班级教学的在线学习系统：课程（视频 / 讲义 / 视频笔记）＋ 班级（公告 / 作业 / 讨论 / 签到 / 成绩册）＋ 考试中心 ＋ AI 能力（答疑 RAG / 出题 / 异步批改 / 知识库）＋ 通知中心。

- 后端：Go 1.27.2 + [go-zero](https://go-zero.dev) 1.10.3，`handler → logic → model` 三层，按业务域分包
- 前端：Vue 3 + Vite + Element Plus，统一请求层、路由守卫、AI 流式进度
- 数据库：MySQL 8.4（Flyway 版本化迁移 V1..V26）／会话与队列：Redis 7.4／AI 知识库：MongoDB 7（文档·切片·原文件 GridFS）
- 向量库：Chroma（按课程隔离，抽象层可换 Milvus）
- 文件：MinIO 私有桶 + 预签名；ffmpeg 转 faststart；LibreOffice 转 PDF 预览
- AI 服务：FastAPI + LangGraph + DashScope（qwen-plus / text-embedding-v3），HTTP 进程与批改 worker 分离
- 部署：Docker Compose（mysql / redis / minio / mongo / flyway / backend / frontend / ai-service / ai-worker）

> 结构目标：这是一个**可演进的单体**。业务按 `internal/app/<context>` 切分，
> 纯规则沉淀到 `internal/domain`，技术设施收敛到 `internal/platform`，入口在 `cmd/api`，
> 为后续拆分微服务留好边界。详见 [docs/architecture.md](docs/architecture.md) 与
> [docs/microservices.md](docs/microservices.md)。

---

## 功能模块（与当前 main 分支一致）

| 模块 | 能力 | 关键实现 |
|---|---|---|
| 课程中心 | 新建 / 编辑 / 删除课程，封面上传，多视频上传，讲义上传与在线预览 | `internal/app/course`；MinIO 私有桶、ffmpeg `-movflags +faststart`、LibreOffice 转 PDF |
| 学习页 | 视频播放、断点续播、真实观看时长、视频笔记 | `video_progress`、V13 `video_notes`、`CourseWatchView` |
| 班级中心 | 建班、邀请码加入、成员管理、绑定 / 解绑课程、管理员班级视图 | `internal/app/class`、V5 / V8；删班级联清理 |
| 教学互动 | 公告（已读 / 未读统计、发布推送）、讨论区、5 分钟限时签到 | V7 / V9 / V10 / V17 |
| 作业 | 客观题自动判分、附件提交、逐份批阅（SpeedGrader）、AI 初评 | V11 `assignment_questions`、V25 `assignment_ai_grades`、V26 `submission_question_scores` |
| 考试中心 | 出题 / 草稿 / 交卷 / 客观题判分 / 主观题手批 / AI 初评 / 自动收卷 | `internal/app/exam`、V15 / V18 / V19 / V20 |
| 成绩册 | 作业 / 考试 / 签到 / 视频四类得分率加权，CSV 导出 | `internal/domain/gradebook`、V12 / V20 |
| 通知中心 | 站内通知、未读计数、全部已读、WebSocket 实时推送、渠道偏好 | V16 `notifications`、V23 `notification_prefs`、`platform/ws` |
| AI 能力 | 答疑 RAG（SSE）、按知识点出题、作业 / 考试异步初评、会话历史、知识库覆盖度 | `ai-service`；Go 侧 `/api/ai/*` 反向代理白名单 |
| 个人中心 | 资料 / 改密、我的作业、我的考试、站内消息、通知偏好 | `ProfileView`、`/api/my/*`、`/api/notifications` |
| 管理后台 | 用户（增删改 / 重置密码 / 角色）、课程、班级 | `/admin/users`、`/admin/courses`、`/admin/classes` |
| 工程与压测 | Docker 全栈、Flyway、CI、Node 发压器与压测报告 | `docker-compose*.yml`、`.github/workflows/ci.yml`、`load-test/` |

---

## 快速开始

```bash
cp .env.example .env          # 填入数据库/Redis/MinIO 密码
docker compose up -d --build  # 首次构建会执行 Flyway 迁移（V1..V26）
```

| 服务 | 地址 | 说明 |
|---|---|---|
| 前端 | <http://localhost:8088> | nginx 托管，`/api`、`/ws` 反代到后端 |
| 后端 | <http://localhost:8888> | go-zero HTTP 服务 |
| 健康检查 | <http://localhost:8888/healthz> | 只看进程存活（容器 liveness） |
| 就绪检查 | <http://localhost:8888/readyz> | MySQL / Redis 可用性（readiness，失败 503） |
| MinIO 控制台 | <http://localhost:9021> | 账号见 `.env`；S3 API 在 9020 |
| MySQL / Redis | `127.0.0.1:3316` / `127.0.0.1:6381` | 本机若已占用默认端口，用映射端口避免冲突 |
| ai-service | 容器内网 `:8000` | 不映射宿主机，外部只能经 Go 的 `/api/ai/*` 访问 |
| MongoDB | 容器内网 `:27017` | 只给 ai-service 用，不对外暴露 |

预置账号：`admin / 123456`。**首次登录会强制要求改密**（`users.must_change_password`），
改完之前顶栏会一直提示。

### 本地开发（不用容器跑应用）

```bash
# 1. 起依赖（MySQL 3316 / Redis 6381 / MinIO 9020 / Flyway 迁移）
docker compose up -d mysql redis minio minio-init flyway

# 2. 后端（环境变量见 backend/etc/*.yaml）
cd backend && go run ./cmd/api -f etc/learningplatform-api.yaml

# 3. 前端（Vite 把 /api 代理到 :8888）
cd frontend && npm install && npm run dev

# 4. AI 服务（可选，不配 Key 时 AI 菜单自动置灰）
cd ai-service && pip install -r requirements.txt
uvicorn app.main:app --reload --port 8000
python -m app.tasks.worker          # 批改消费者另开一个终端
```

AI 服务未配置时，`GET /api/ai/status` 返回 `enabled=false` 和人话原因，
前端把 AI 入口置灰，而不是点了才报错；课程、班级、考试等主链路不受影响。

---

## 目录结构

```text
backend/
  cmd/api/                 # 服务入口（拆服务时每个服务一个 cmd/<name>）
  internal/
    app/                   # 业务边界，每个包自带 handler + logic
      user/                # 登录、注册、会话、密码、角色申请
      course/              # 课程、封面/视频/讲义、学习进度、视频笔记、媒体访问策略
      class/               # 班级、公告、作业、讨论、签到、成绩册、管理后台
      exam/                # 考试中心（出题、草稿、交卷、判分、批改、AI 初评）
      notification/        # 通知偏好 REST + WebSocket 推送
    domain/                # 纯规则，零 IO 依赖，可独立单测
      authz/               # 能力矩阵（Moodle capability 模型）
      gradebook/           # 成绩册口径（Canvas 加权）
      examrules/           # 考试时间窗口 / 答案可见性
    platform/              # 技术设施
      oss/ quota/ middleware/ response/ concurrency/ ws/ aiservice/
    model/                 # MySQL 数据访问（手写 SQL）
    config/ svc/           # 配置与依赖装配
    handler/               # 路由汇总 + 健康检查 + /api/ai/* 反向代理
  deploy/sql/migration/    # Flyway V1..V26
frontend/
  src/api/client.js        # 统一请求层：token 注入、401 跳登录、错误文案
  src/api/index.js         # 通用接口封装（rawClient，兼容 `res.data` 用法）
  src/api/ai.js            # AI 接口：chat SSE、出题进度流、初评
  src/utils/               # 课程创建流程、通知 WebSocket、出题进度
  src/views/               # 页面（含 SpeedGraderView、AiChatView、AiGenerateView、admin/*）
  src/components/class/    # 班级页签（作业/成绩/签到/公告/讨论）
  tests/                   # node --test 纯逻辑单测（20 个测试文件）
ai-service/                # AI 子服务：答疑 RAG / 出题 / 异步批改 / MongoDB 知识库
  app/api/                 # chat / generate / grade / session / kb
  app/graph/               # LangGraph 4 节点：rewrite→retrieve→prompt→llm
  app/kb/ rag/ questions/  # 知识库、向量与解析、出题域
  app/tasks/               # worker / grade_worker / grade_store
  tests/                   # pytest（不打外网、不连真实 Mongo/Redis）
docs/                      # 架构、微服务拆分、性能说明、结构说明
deploy/                    # 服务器部署手册、nginx 配置、初始化脚本（尚未上线使用）
load-test/                 # Node 发压器 + 场景配置 + 原始结果（2026-11-09 / 2026-11-15）
load-test-report/          # 历史压测综合报告
scripts/                   # 本地一键启动、重建、部署、E2E 脚本
```

---

## 授权模型

所有权限判定收敛到 `internal/domain/authz` 的一张静态矩阵：

```go
actor := authz.Actor{Username: u.Username, Role: u.Role, ClassRole: classRole}
if err := actor.Require(authz.CourseDelete, authz.Resource{OwnerUsername: c.CreatedBy}); err != nil {
    return err   // 403 + 可读文案
}
```

| 能力 | 谁有 |
|---|---|
| `course.edit` / `course.delete` / `course.upload` | **课程创建者本人** 或管理员（其他老师也不行） |
| `class.delete` | **班级创建者本人** 或管理员（同班其他老师不行） |
| `class.manage` / `assignment.grade` / `exam.grade` / `grade.view.all` | **班内老师** 或管理员（全局 teacher 但不在班里 → 拒绝） |
| `course.create` / `class.create` | 全站老师或管理员 |
| 未声明的能力 | 默认拒绝（fail-closed） |

课程写操作在 HTTP 边界统一过 `authorizeCourse`，考试 / 班级在 logic 第一行校验。
**媒体（视频 / 讲义 / 预览）不等同于课程写权限**：它按"是不是学习者"判定，见下一节。

班级的删除与退出：

- 删班（`DELETE /api/class/:id`）会级联清理作业与提交、考试与答卷、成绩册、讨论、签到、
  邀请码、公告、成员关系和指向该班的站内消息，全部在一个事务里；返回各表清理条数。
- 学生退出（`POST /api/class/:id/quit`）只删 `class_members` 一行，**已交作业与成绩保留**；
  老师与管理员不能走这条路（老师的正解是删班或由管理员处理）。
- 管理员在 `/admin/classes` 看全部班级（含成员 / 课程 / 作业 / 考试统计）、改名称年级专业、
  管理成员、删班。

---

## 媒体访问（私有桶的正确用法）

`MINIO_PRIVATE=true` 时，媒体对象不再允许匿名读，接口返回 30 分钟预签名 URL。

关键点：签名只在**鉴权通过后**发生。规则（`course/media_access.go`）：

| 访问者 | 独立课程（未绑定班级） | 班级课程 |
|---|---|---|
| 未登录 | 拒绝（401，跳登录） | 拒绝 |
| 管理员 / 创建者 | 允许 | 允许 |
| 班内学生 | 允许 | 允许 |
| 其他登录用户 | 允许（公开课语义） | **拒绝**（403） |

`/api/course/detail/:id` 本身公开（首页 / 分享链接可打开），但未获授权时视频 `url` 为空，
并返回 `canWatch=false` + `mediaReason`，前端渲染"登录 / 加入班级后学习"。

---

## 学习进度与视频笔记

`video_progress(user_id, video_id)` 记录：

| 字段 | 含义 |
|---|---|
| `position_seconds` | 上次播放位置（续播） |
| `watched_seconds` | 累计**真实播放**秒数（拖动不计） |
| `duration_seconds` | 视频总时长（播放器上报） |
| `completed` | 累计观看 ≥ 时长 90% |

前端每 8 秒上报一次增量，离开页面用 `fetch(keepalive)` 补报。进度按用户隔离。

视频笔记落在 `video_notes`（V13），按用户 + 课程 + 视频隔离：

- 写笔记：`POST /api/course/note`
- 读某视频笔记：`GET /api/course/note/:courseId/:videoId`
- 课程笔记汇总：`GET /api/course/notes/summary/:courseId`

播放页侧边栏记笔记，点击笔记可跳回对应时间点；笔记当前只在课程内使用，
AI 答疑的知识库清单不包含笔记内容。

---

## 成绩册口径（对标 Canvas gradebook）

```
作业得分率 = Σ作业得分 / Σ作业满分 × 100
考试得分率 = Σ考试得分 / Σ考试满分 × 100
签到得分率 = 已签次数 / 班级签到场次 × 100
视频得分率 = 已看完视频数 / 班级绑定课程视频总数 × 100
加权总分   = Σ(各类得分率 × 该类权重) / Σ权重        （空分类不占权重）
```

- 分数在**提交 / 批改时**写入 `grade_scores`（每作业 / 考试一条 `grade_items`），查询只做聚合；
- 入口：`GET /api/class/:id/grades`（老师）、`GET /api/class/:id/grades/export`（CSV，带 BOM）。

---

## 作业与考试

作业侧：

- 客观题（单选 / 多选 / 填空，以及附件 / 简答以外的题型）在 `assignment_questions`（V11），
  学生提交后后端比对答案算 `autoScore`；`essay` 主观题不参与自动判分；
- 主观题与附件走人工批改，逐份批阅页对标 Canvas SpeedGrader；
- AI 初评只写**建议分**到 `assignment_ai_grades`（V25），成绩册仍由老师落笔；
  解析失败的作答进 `ai:grade:manual` 人工队列，不会直接写 0 分；
- V26 `submission_question_scores` 保存每题得分，便于逐题回溯与重算。

考试侧（服务端权威）：

- 个人截止时间 = `min(考试结束, 本人开考时间 + 时长)`，服务端返回 `deadlineAt`，前端只展示；
- 交卷是**一次性**的：`exam_submissions.status` 从 `started` → `submitted`，重复提交返回 409；
- **作答草稿自动保存**：前端每 15 秒上报一次（内容没变不发），
  后台超时收卷按最后一份草稿判分 —— 断网 / 崩溃不再等于 0 分；
- 标准答案**只在整场考试结束后**下发；
- 后台任务每分钟扫描超时答卷强制收卷（`status=expired`）；
- 老师手批分数限制在 `0..满分`，避免"录 999 分"把加权总分带飞；
- `allow_late` + `late_deadline` 控制补交窗口；`tab_limit` 由前端上报切屏次数、服务端累加；
- AI 初评：`POST /api/exam/submission/:sid/ai-grade`，同样只给建议分，失败进人工队列。

---

## 通知中心

- 表：`notifications`（V16，`user_id/type/title/content/link_url/read`）、
  `notification_prefs`（V23，按用户 + 渠道 upsert）。
- REST：
  - `GET /api/notifications`：列表 + 未读数
  - `POST /api/notifications/:id/read`
  - `POST /api/notifications/read-all`
  - `GET /api/notification-prefs` / `POST /api/notification-prefs`：`inapp` / `email` 开关
- 实时：`GET /ws/notifications?token=<会话 token>`。浏览器握手不能带自定义头，
  token 走 query，服务端按 Redis 会话键校验；连接由 `platform/ws.Hub` 统一管理，
  公告发布、教师申请、角色变更会推送未读提示。
- 当前 `email` 只是偏好存储，**邮件实际发送尚未接入**；站内信是唯一已实现的渠道。

---

## AI 模块

AI 是独立服务，Go 只做鉴权后转发，不承载模型逻辑。四块能力：

1. **答疑 RAG**（同步 SSE）：`POST /api/ai/chat`，LangGraph 四节点（改写 → 混合召回 → Prompt → LLM），
   Chroma 向量 + MongoDB 中文 bigram 词法 RRF 融合，带引用溯源；
2. **AI 出题**：`POST /api/ai/generate`。默认按知识点检索简体中文权威资料作依据，
   四层随机化（命题蓝图 / 采样抖动 / 三层去重 / 后处理），支持 `Accept: text/event-stream` 阶段进度流；
   可选英文通识题库只用于课堂热身；
3. **异步批改**：`POST /api/ai/grade` **不在浏览器代理白名单里**，载荷由 Go 从数据库组装，
   `ai-worker` 消费 Redis 队列，回写"建议分"；
4. **会话与知识库**：`GET /api/ai/session/history`、`DELETE /api/ai/session`、
   `GET /api/ai/kb/status`（只返回聚合数字，不含提问原文）。

Go 侧代理白名单（`internal/handler/ai_proxy.go`）：

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/ai/chat` | 答疑，SSE |
| POST | `/api/ai/generate` | 出题，JSON / SSE |
| GET | `/api/ai/session/history` | 对话历史 |
| DELETE | `/api/ai/session` | 清空对话 |
| GET | `/api/ai/kb/status` | 知识库覆盖度 |

代理会**剥掉**客户端伪造的 `X-User-Id` / `X-Role`，用 JWT 解析出的可信身份覆盖，
再注入 `X-Internal-Token`。超时自外向内逐级放宽：nginx 300s（AI 流 600s）
> go-zero 180s > 代理非 SSE 165s > 前端 axios 150s > LLM 单次 60s；
SSE 只卡 10s 响应头，普通 JSON 不卡响应头，避免中文出题在第 10 秒被误判 503。

知识库细节、灌库流程、覆盖度闭环与未完成项见 [ai-service/README.md](ai-service/README.md)。

---

## 上传：鉴权 + 私有化 + 配额

| 层 | 规则 | 实现 |
|---|---|---|
| 顺序 | **先鉴权、再解析 body**（`courseId` 放 query） | `course/handler/guard.go` |
| 身份 | 只有课程创建者（或管理员）能往这门课传文件 | `course.Authorize` + `authorizeCourse` |
| 单文件 | 封面 5MB / 视频 500MB / 讲义 50MB | `Upload.*Bytes` |
| 总量 | 每用户每日 2GB（`Upload.DailyQuotaBytesPerUser`） | `platform/quota`，Redis 计数，超限 **413** |
| 请求体 | 默认 1MB，仅上传路由放宽 | `platform/middleware/bodylimit.go` |

配额 key 形如 `upload:quota:20261027:42`（48h TTL，跨日自动清零）；
上传失败会调用 `quota.Release` 归还，不会因为一次失败吃光当天额度。

对象生命周期：孤儿清理器每 10 分钟扫一次，只删**白名单前缀**下、超过安全窗口、
且未被任何课程引用的对象。引用集合覆盖封面 / 缩略图 / 视频（含 faststart 源文件）/ 讲义 / 预览缓存
（`course/objectrefs.go`，有回归测试）。

分片直传接口已实现（`multipart/init`、`sign`、`complete`、`abort`、`status`），
前端目前仍走 Go 中转，下一步切到浏览器直传 MinIO。

---

## 统一响应与请求层

- 错误统一为 `{code, message}`（HTTP 状态码即业务码），成功沿用既有"裸 data"结构以兼容前端；
- 前端 `src/api/client.js` 只有一份拦截器：自动带 token、401 清会话并跳登录、统一提取中文文案；
  - `client`：新代码用，自动解包 `{code, message, data}`；
  - `rawClient`：兼容老页面 `res.data.xxx` 的写法；
- 路由守卫：`meta.requiresAuth` 的页面未登录直接跳 `/login?redirect=...`，另有 404 兜底；
- 会话信息（角色、是否必须改密）以服务端 `/api/user/info` 为准，管理员改角色后刷新即生效。

---

## 性能与并发

后端按"先修查询、再谈并发"的顺序做了一轮规范化改造，规则与参考项目见
[docs/performance.md](docs/performance.md)。已落地的优化：

- 公告已读、成绩册聚合从 N+1 / 相关子查询改为批量聚合：公告往返 20→1，
  成绩册查询 268~296ms → 59~107ms（120 人 / 4560 条成绩，EXPLAIN ANALYZE）；
- 并发统一走 `internal/platform/concurrency.ForEach`：有界（上限明确）、可取消、panic 不炸进程；
- 最高频写路径（视频进度上报）3 次往返 → 2 次；超时收卷改为批量取数 + 4 路有界并发；
- 登录会话携带 `userId`，班级模块每个请求少一次 `users` 查询（老会话自动回落）；
- 迁移 `V22` 按真实 SQL 补 7 个复合索引（幂等）。

### 本机压测基线（2026-11-15，最新一次）

完整数据、场景配置、原始结果与复现命令见
[load-test/2026-11-15/REPORT.md](load-test/2026-11-15/REPORT.md)：

| 场景 | 并发 | QPS | P50 | P95 | P99 | 成功率 |
|---|---|---|---|---|---|---|
| 混合读（课程/详情/公告/作业/班级） | 16 | 3071.91 | 5.11 ms | 7.81 ms | 9.32 ms | 100% |
| 混合读 | 32 | 2855.63 | 11.16 ms | 16.13 ms | 19.59 ms | 100% |
| 混合读 | 64 | 2510.80 | 25.76 ms | 29.39 ms | 32.08 ms | 100% |
| 写笔记 | 16 | 1226.89 | 10.77 ms | 24.15 ms | 29.03 ms | 100% |
| 写播放进度 | 16 | 821.04 | 17.20 ms | 35.60 ms | 50.98 ms | 100% |
| 读写混合 | 32 | 2321.43 | 13.46 ms | 19.78 ms | 22.70 ms | 100% |

结论摘要：读峰值出现在 16 并发，32 / 64 并发逐步回落，但尾延迟控制较好（64 并发 P99 32ms）；
写路径没有限流，QPS 随并发上升而 P95 同步翻倍；班级详情（285 QPS）是最慢读接口，
因为做了多次关联查询。压测为本机 loopback、每档 20 秒，只作趋势与回归基线，不能直接当容量承诺。

---

## 测试

```bash
cd backend  && go vet ./... && go test ./...     # 单元 + 集成（无依赖时自动 skip）
cd frontend && npm test                          # 路由守卫 / API 封装 / AI / 通知 / 上传
cd frontend && npm run build                     # 构建即校验所有 .vue 模板/语法
cd ai-service && python -m pytest tests -q       # 253 个用例，全部 mock 外部依赖
```

重点覆盖：

- `internal/domain/authz`：越权矩阵（别人能不能删我的课、全局老师能不能批别班的卷）
- `internal/domain/gradebook`：权重、封顶、空分类不占权重、等级换算
- `internal/domain/examrules`：个人截止时间、补交窗口、答案可见性、自动收卷
- `internal/app/course`：媒体访问规则、对象引用收集、孤儿清理白名单（含讲义回归测试）
- `internal/app/exam/logic`：草稿判分与超时收卷（集成测试，校验成绩册落库）
- `internal/app/class`：客观题判分、题目得分、AI 初评回写
- `internal/platform/middleware`：请求体限长（413 / 上传放宽 / chunked）
- `ai-service`：混合召回、故障降级、出题随机性与中文质检、灌库幂等、覆盖度接口

集成测试默认连 `127.0.0.1:3316` / `6381`，可用 `TEST_MYSQL_DSN`、`TEST_REDIS_PASS` 覆盖。
CI 用 service containers 真跑，见 `.github/workflows/ci.yml`。

压测：旧版 Go 发压器 `cd load-test && LP_TOKEN=<登录 token> go run main.go`；
最新一轮用 Node 发压器 `load-test/2026-11-15/bench.js`，历史结果见
[load-test-report/REPORT.md](load-test-report/REPORT.md)。

---

## 安全说明

- `.env`、`ai-service/.env` 均被 `.gitignore` 忽略；模板见 `.env.example`、`ai-service/.env.example`。
- 曾在 `ai-service/.env` 出现过的第三方 Key 应视为已泄露，需到控制台作废并重新生成。
- 密码 bcrypt 存储；历史明文账号在首次登录成功后自动升级；
  初始账号 / 管理员重置的密码带 `must_change_password`，改密前前端持续提示。
- 媒体访问失败按 fail-closed 处理：数据库抖动时拒绝而不是放行。
- AI 服务除 `/health`、`/readyz` 外全部要求 `X-Internal-Token`；
  `INTERNAL_AUTH_ENABLED=true` 且密钥为空时直接 403，不会"忘了配"就退化成不校验。
- Go 代理会清除客户端伪造的 `X-User-Id` / `X-Role`，身份只认 JWT；
  批改入口 `POST /api/ai/grade` 不在浏览器白名单，只能由服务端调用。
- 通知 WebSocket 的 `CheckOrigin` 目前开发期放开（返回 true），**上线前必须改为校验前端域名**。

---

## 已知限制 / 后续方向

1. **未上线公网**：当前只有本地开发环境；服务器部署方案（`deploy/server/README.md`、
   `docker-compose.server.yml`）已准备但未执行，域名、HTTPS、备份、监控告警都还没接。
2. **AI 知识库仍有缺口**：视频本体转写（ffmpeg 抽音频 + STT）还是 TODO，现在只索引视频标题；
   老式 `.doc/.ppt` 与扫描件 OCR 未支持；这些都会在 `kb_documents.status=fallback` 如实标注。
   配套 `.srt/.vtt` 字幕已支持解析，是当前不等 ASR 的折中方案。
3. **学习总结未接入**：`学习总结` 独立分支实现过 `ai-service/app/api/summary.py`，
   但当前 main 的 ai-service 只注册 chat/generate/session/grade/kb 五个路由，前端也没有入口，
   属于"历史实现、未上线"，不要当成已交付能力。
4. **视频直传**：分片直传接口（init/sign/complete）已实现，前端仍走 Go 中转；下一步切到浏览器直传 MinIO。
5. **数据层**：暂无外键与软删除；删除课程依赖应用层事务 + 孤儿清理，跨服务时要改成事件。
6. **观测**：已有 request id 与健康检查；尚未接 Prometheus 指标、慢查询日志与告警。
7. **课程存储**：`course.Store` 仍是包级单例（兼容历史测试用内存实现）；拆服务前应改为显式注入。
8. **通知渠道**：只有站内信真实可用，`email` 偏好只存开关，没有发信实现；
   AI 出题依赖外网中文资料，4s 超时或源不可达时自动回退纯模型出题，`meta.material_status` 如实标注。
9. **压测报告**：本机 loopback、短时（每档 20 秒）结果，只作趋势参考；要作为容量依据需补固定数据量与 30 分钟长稳。
