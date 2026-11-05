# 学习平台（Learning Platform）

面向班级教学的在线学习系统：课程（视频/讲义）＋ 班级（公告/作业/讨论/签到）＋ 考试中心 ＋ 成绩册。

- 后端：Go 1.27 + [go-zero](https://go-zero.dev) 1.10，`handler → logic → model` 三层
- 前端：Vue 3 + Vite + Element Plus
- 数据库：MySQL 8.4（Flyway 版本化迁移）／缓存与会话：Redis 7
- 文件：MinIO（私有桶 + 预签名），ffmpeg 转 faststart，LibreOffice 转 PDF 预览
- 部署：Docker Compose 一键起（mysql / redis / minio / flyway / backend / frontend）

> 结构目标：这是一个**可演进的单体**。业务按 `internal/app/<context>` 切分，
> 纯规则沉淀到 `internal/domain`，技术设施收敛到 `internal/platform`，入口在 `cmd/api`，
> 为后续拆分微服务留好边界。详见 [docs/architecture.md](docs/architecture.md) 与
> [docs/microservices.md](docs/microservices.md)。

---

## 快速开始

```bash
cp .env.example .env          # 填入数据库/Redis/MinIO 密码
docker compose up -d --build  # 首次构建会执行 Flyway 迁移（V1..V21）
```

| 服务 | 地址 | 说明 |
|---|---|---|
| 前端 | <http://localhost:8088> | nginx 托管，`/api` 反代到后端 |
| 后端 | <http://localhost:8888> | go-zero HTTP 服务 |
| 健康检查 | <http://localhost:8888/healthz> | 进程存活（容器 liveness） |
| 就绪检查 | <http://localhost:8888/readyz> | MySQL / Redis 可用性（readiness） |
| MinIO 控制台 | <http://localhost:9021> | 账号见 `.env` |

预置账号：`admin / 123456`。**首次登录会强制要求改密**（`users.must_change_password`），
改完之前顶栏会一直提示。

### 本地开发（不用容器跑应用）

```bash
# 1. 起依赖（MySQL 3316 / Redis 6381 / MinIO 9020）
docker compose up -d mysql redis minio minio-init flyway

# 2. 后端（环境变量见 backend/etc/*.yaml）
cd backend && go run ./cmd/api -f etc/learningplatform-api.yaml

# 3. 前端（Vite 把 /api 代理到 :8888）
cd frontend && npm install && npm run dev
```

---

## 目录结构

```text
backend/
  cmd/api/                 # 服务入口（拆服务时每个服务一个 cmd/<name>）
  internal/
    app/                   # 业务边界，每个包自带 handler + logic
      user/                # 登录、会话、密码
      course/              # 课程、封面/视频/讲义、学习进度、媒体访问策略
      class/               # 班级、公告、作业、讨论、签到、成绩册
      exam/                # 考试中心（出题、草稿、交卷、判分、批改）
    domain/                # 纯规则，零 IO 依赖，可独立单测
      authz/               # 能力矩阵（Moodle capability 模型）
      gradebook/           # 成绩册口径（Canvas 加权）
      examrules/           # 考试时间窗口 / 答案可见性
    platform/              # 技术设施
      oss/ quota/ middleware/ response/
    model/                 # MySQL 数据访问（手写 SQL）
    config/ svc/           # 配置与依赖装配
    handler/               # 路由汇总 + 健康检查
  deploy/sql/migration/    # Flyway V1..V21
frontend/
  src/api/client.js        # 统一请求层：token 注入、401 跳登录、错误文案
  src/api/index.js         # 接口封装（rawClient，兼容 `res.data` 用法）
  src/views/               # 页面（含 SpeedGraderView 逐份批阅）
  src/components/class/    # 班级页签（作业/成绩/签到/公告/讨论）
  tests/                   # node --test 纯逻辑单测
ai-service/                # 下一步：AI 答疑服务（当前只有契约与环境模板）
docs/                      # 架构与拆分说明
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
| `class.manage` / `assignment.grade` / `exam.grade` / `grade.view.all` | **班内老师** 或管理员（全局 teacher 但不在班里 → 拒绝） |
| `course.create` / `class.create` | 全站老师或管理员 |
| 未声明的能力 | 默认拒绝（fail-closed） |

课程写操作在 HTTP 边界统一过 `authorizeCourse`，考试/班级在 logic 第一行校验。
**媒体（视频/讲义/预览）不等同于课程写权限**：它按"是不是学习者"判定，见下一节。

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

`/api/course/detail/:id` 本身公开（首页/分享链接可打开），但未获授权时视频 `url` 为空，
并返回 `canWatch=false` + `mediaReason`，前端渲染"登录/加入班级后学习"。

---

## 成绩册口径（对标 Canvas gradebook）

```
作业得分率 = Σ作业得分 / Σ作业满分 × 100
考试得分率 = Σ考试得分 / Σ考试满分 × 100
签到得分率 = 已签次数 / 班级签到场次 × 100
视频得分率 = 已看完视频数 / 班级绑定课程视频总数 × 100
加权总分   = Σ(各类得分率 × 该类权重) / Σ权重        （空分类不占权重）
```

- 分数在**提交/批改时**写入 `grade_scores`（每作业/考试一条 `grade_items`），查询只做聚合；
- 入口：`GET /api/class/:id/grades`（老师）、`GET /api/class/:id/grades/export`（CSV，带 BOM）。

---

## 学习进度（对标 Open edX）

`video_progress(user_id, video_id)` 记录：

| 字段 | 含义 |
|---|---|
| `position_seconds` | 上次播放位置（续播） |
| `watched_seconds` | 累计**真实播放**秒数（拖动不计） |
| `duration_seconds` | 视频总时长（播放器上报） |
| `completed` | 累计观看 ≥ 时长 90% |

前端每 8 秒上报一次增量，离开页面用 `fetch(keepalive)` 补报。进度按用户隔离。

---

## 考试规则（服务端权威）

- 个人截止时间 = `min(考试结束, 本人开考时间 + 时长)`，服务端返回 `deadlineAt`，前端只展示；
- 交卷是**一次性**的：`exam_submissions.status` 从 `started` → `submitted`，重复提交返回 409；
- **作答草稿自动保存**：前端每 15 秒上报一次（内容没变不发），
  后台超时收卷按最后一份草稿判分 —— 断网/崩溃不再等于 0 分；
- 标准答案**只在整场考试结束后**下发；
- 后台任务每分钟扫描超时答卷强制收卷（`status=expired`）；
- 老师手批分数限制在 `0..满分`，避免"录 999 分"把加权总分带飞；
- `allow_late` + `late_deadline` 控制补交窗口；`tab_limit` 由前端上报切屏次数、服务端累加。

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
且未被任何课程引用的对象。引用集合覆盖封面/缩略图/视频（含 faststart 源文件）/讲义/预览缓存
（`course/objectrefs.go`，有回归测试）。

---

## 统一响应与请求层

- 错误统一为 `{code, message}`（HTTP 状态码即业务码），成功沿用既有"裸 data"结构以兼容前端；
- 前端 `src/api/client.js` 只有一份拦截器：自动带 token、401 清会话并跳登录、统一提取中文文案；
  - `client`：新代码用，自动解包 `{code, message, data}`；
  - `rawClient`：兼容老页面 `res.data.xxx` 的写法；
- 路由守卫：`meta.requiresAuth` 的页面未登录直接跳 `/login?redirect=...`，另有 404 兜底；
- 会话信息（角色、是否必须改密）以服务端 `/api/user/info` 为准，管理员改角色后刷新即生效。

---

## 性能与并发（B 端）

后端按"先修查询、再谈并发"的顺序做了一轮规范化改造，规则、实测数据、参考项目与复现方法见
[docs/performance.md](docs/performance.md)。要点：

- 公告已读、成绩册聚合从 N+1/相关子查询改为批量聚合：公告往返 20→1，
  成绩册查询 268~296ms → 59~107ms（120 人 / 4560 条成绩，EXPLAIN ANALYZE）；
- 并发统一走 `internal/platform/concurrency.ForEach`：有界（上限明确）、可取消、panic 不炸进程；
- 最高频写路径（视频进度上报）3 次往返 → 2 次；超时收卷改为批量取数 + 4 路有界并发；
- 登录会话携带 `userId`，班级模块每个请求少一次 `users` 查询（老会话自动回落）；
- 迁移 `V22` 按真实 SQL 补 7 个复合索引（幂等）；压测造数脚本见
  `scripts/perf/seed_gradebook_bench.sql`。

---

## 测试

```bash
cd backend  && go vet ./... && go test ./...     # 单元 + 集成（无依赖时自动 skip）
cd frontend && npm test                          # 路由守卫 / API 封装 / 创建课程流程
cd frontend && npm run build                     # 构建即校验所有 .vue 模板/语法
```

重点覆盖：

- `internal/domain/authz`：越权矩阵（别人能不能删我的课、全局老师能不能批别班的卷）
- `internal/domain/gradebook`：权重、封顶、空分类不占权重、等级换算
- `internal/domain/examrules`：个人截止时间、补交窗口、答案可见性、自动收卷
- `internal/app/course`：媒体访问规则、对象引用收集、孤儿清理白名单（含讲义回归测试）
- `internal/app/exam/logic`：草稿判分与超时收卷（集成测试，校验成绩册落库）
- `internal/platform/middleware`：请求体限长（413 / 上传放宽 / chunked）

集成测试默认连 `127.0.0.1:3316` / `6381`，可用 `TEST_MYSQL_DSN`、`TEST_REDIS_PASS` 覆盖。
CI 用 service containers 真跑，见 `.github/workflows/ci.yml`。

压测：`cd load-test && LP_TOKEN=<登录 token> go run main.go`（token 也可用 `-token` 传入，不再写死在代码里），历史结果见 [load-test-report/REPORT.md](load-test-report/REPORT.md)。

---

## 安全说明

- `.env`、`ai-service/.env` 均被 `.gitignore` 忽略；模板见 `.env.example`、`ai-service/.env.example`。
- 曾在 `ai-service/.env` 出现过的第三方 Key 应视为已泄露，需到控制台作废并重新生成。
- 密码 bcrypt 存储；历史明文账号在首次登录成功后自动升级；
  初始账号/管理员重置的密码带 `must_change_password`，改密前前端持续提示。
- 媒体访问失败按 fail-closed 处理：数据库抖动时拒绝而不是放行。

---

## 已知限制 / 后续方向

1. **AI 模块**：`ai-service/` 目前只有契约与环境模板，计划先做答疑、再做作业初评（HTTP 回调主服务）。
2. **视频直传**：分片直传接口（init/sign/complete）已实现，前端仍走 Go 中转；下一步切到浏览器直传 MinIO。
3. **数据层**：暂无外键与软删除；删除课程依赖应用层事务 + 孤儿清理，跨服务时要改成事件。
4. **观测**：已有 request id 与健康检查；下一步接 go-zero 的 Prometheus 指标、慢查询日志与告警。
5. **课程存储**：`course.Store` 仍是包级单例（兼容历史测试用内存实现）；拆服务前应改为显式注入。
6. **压测报告**：历史数据来自小数据集与 5 秒短压，仅作趋势参考；要作为容量依据需补固定数据量与 30 分钟长稳。
