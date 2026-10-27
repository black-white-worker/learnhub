# 学习平台（Learning Platform）

面向班级教学的在线学习系统：课程（视频/讲义）＋ 班级（公告/作业/讨论/签到）＋ 考试中心 ＋ 成绩册。

- 后端：Go 1.27 + [go-zero](https://go-zero.dev) 1.10，handler → logic → model 三层
- 前端：Vue 3 + Vite + Element Plus
- 数据库：MySQL 8.4（Flyway 版本化迁移）／缓存与会话：Redis 7
- 文件：MinIO（封面、视频、讲义），ffmpeg 转码 faststart，LibreOffice 转 PDF 预览
- 部署：Docker Compose 一键起（mysql / redis / minio / flyway / backend / frontend）

> 本 README 记录的是「一次系统性整改之后」的架构与约定。
> 整改重点：权限收口、考试防作弊闭环、成绩册口径、学习进度、老师批阅效率、统一请求层。

---

## 快速开始

```bash
cp .env.example .env          # 填入数据库/Redis/MinIO 密码
docker compose up -d --build  # 首次构建；之后可用 scripts/start.ps1 增量启动
```

- 前端：<http://localhost:8088>
- 后端：<http://localhost:8888>
- MinIO 控制台：<http://localhost:9021>

预置账号：`admin / 123456`（首次登录会自动把明文密码升级为 bcrypt）。

### 本地开发（不用容器）

```bash
# 1. 起依赖
docker compose up -d mysql redis minio

# 2. 后端（需要 MYSQL_DSN / REDIS_HOST 等环境变量，见 backend/etc/*.yaml）
cd backend && go run learningplatform.go -f etc/learningplatform-api.yaml

# 3. 前端（/api 代理到 :8888）
cd frontend && npm install && npm run dev
```

---

## 目录结构

```
backend/
  internal/
    authz/            # 授权能力矩阵（Moodle capability 模型的落地）
    gradebook/        # 成绩册口径的纯计算（权重、封顶、等级）
    app/
      user/           # 登录、会话、鉴权中间件
      course/         # 课程、视频、讲义、学习进度
      class/          # 班级、公告、作业、讨论、签到、成绩册
      exam/           # 考试中心（出题、交卷、判分、批改）
        logic/examrules/  # 考试时间规则（纯函数，可单测）
    model/            # 数据访问层（手写 SQL）
    response/         # 统一响应封装
  deploy/sql/migration/  # Flyway V1..V20
frontend/
  src/
    api/client.js     # 统一请求层：token 注入、401 跳登录、错误文案
    views/            # 页面（含 SpeedGraderView 逐份批阅）
    components/class/ # 班级页签（作业/成绩/签到/公告/讨论）
```

---

## 授权模型（改造重点）

所有权限判定收敛到 `internal/authz` 的一张静态矩阵，调用方式是：

```go
actor := authz.Actor{Username: u.Username, Role: u.Role, ClassRole: classRole}
if err := actor.Require(authz.CourseDelete, authz.Resource{OwnerUsername: c.CreatedBy}); err != nil {
    return err   // 返回 403 + 可读文案
}
```

关键规则：

| 能力 | 谁有 |
|---|---|
| `course.edit` / `course.delete` / `course.upload` | **课程创建者本人** 或管理员（其他老师也不行） |
| `class.manage` / `assignment.grade` / `exam.grade` / `grade.view.all` | **班内老师** 或管理员（全局 teacher 但不在班里 → 拒绝） |
| `course.create` / `class.create` | 全站老师或管理员 |
| 未声明的能力 | 默认拒绝（fail-closed） |

> 整改前：课程删除/编辑、考试阅卷都只校验了"已登录"，任何学生都能删别人的课、改别人的分数。
> 现在课程写操作在 HTTP 边界统一过 `authorizeCourse`，考试/班级在 logic 第一行校验。

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
- 入口：`GET /api/class/:id/grades`（老师）、`GET /api/class/:id/grades/export`（CSV，带 BOM）；
- 整改前：视频项写死满分、签到次数×10 不封顶、考试权重被丢弃 —— 总分没有教学含义。

---

## 学习进度（对标 Open edX）

`video_progress(user_id, video_id)` 记录：

| 字段 | 含义 |
|---|---|
| `position_seconds` | 上次播放位置（续播） |
| `watched_seconds` | 累计**真实播放**秒数（拖动不计） |
| `duration_seconds` | 视频总时长（播放器上报） |
| `completed` | 累计观看 ≥ 时长 90% |

前端每 8 秒上报一次增量，离开页面用 `fetch(keepalive)` 补报。
整改前：进度存在 Redis，key 不带用户 id —— 全班共用一个进度条，也无法做学情分析。

---

## 考试规则（服务端权威）

- 个人截止时间 = `min(考试结束, 本人开考时间 + 时长)`，由服务端返回 `deadlineAt`，前端只展示；
- 交卷是**一次性**的：`exam_submissions.status` 从 `started` → `submitted`，重复提交返回 409；
- 标准答案**只在整场考试结束后**下发（考试期间先交卷的学生也看不到）；
- 后台任务每分钟扫描超时答卷强制收卷（`status=expired`）；
- `allow_late` + `late_deadline` 控制补交窗口；`tab_limit` 由前端上报切屏次数、服务端累加，老师端可见。

---

## 上传：鉴权 + 私有化 + 配额

课件的上传/删除是"课程资产"操作，三层防护：

| 层 | 规则 | 实现 |
|---|---|---|
| 身份 | 只有课程创建者（或管理员）能往这门课传文件 | `course.Authorize` + `authorizeCourse` |
| 体积 | 封面 5MB / 视频 500MB / 讲义按配置 | `Upload.*Bytes` |
| 总量 | 每用户每日上传上限（`Upload.DailyQuotaBytesPerUser`，0=不限） | `internal/quota`，Redis 计数，超限 **413** |

配额 key 形如 `upload:quota:20261027:42`（48h TTL，跨日自动清零）；分片直传在 `init` 时按声明大小一次性预占。

私有桶：`MINIO_PRIVATE=true` 后，课程详情/视频播放/讲义列表返回 30 分钟有效预签名 URL。

## 统一响应与请求层

- 错误统一为 `{code, message}`（HTTP 状态码即业务码），成功沿用既有"裸 data"结构以兼容前端；
- 前端 `src/api/client.js` 给 **全局 axios** 挂拦截器：自动带 token、401 清会话并跳登录、
  统一提取中文错误文案；新代码用 `client` 实例（自动解包 `{code,message,data}`）；
- 路由守卫：`meta.requiresAuth` 的页面未登录直接跳 `/login?redirect=...`，另有 404 兜底。

---

## 测试

```bash
cd backend  && go test ./...          # 单元测试；需要 MySQL/Redis 的集成测试会自动 skip
cd frontend && npm test               # 前端 API 单测
cd frontend && npx vite build         # 构建即校验所有 .vue 模板/语法
```

重点覆盖：

- `internal/authz`：越权矩阵（别人能不能删我的课、全局老师能不能批别班的卷）
- `internal/gradebook`：权重、封顶、空分类不占权重、等级换算
- `internal/app/exam/logic/examrules`：个人截止时间、补交窗口、答案可见性、自动收卷
- `internal/app/course`：课程写操作的 owner 校验

CI：`.github/workflows/ci.yml`（go build/vet/test + 前端 build/test）。

---

## 已知限制 / 后续方向

1. **对象存储仍是公开读**：`minio-init` 给 `course/` 前缀开了 anonymous download，
   拿到 URL 的人可以直接下载/盗链。下一步应改为私有桶 + 预签名 GET（读接口按登录态签名）。
2. **角色变更对老会话延迟生效**：角色缓存在 Redis 会话里（24h），改角色后需要重新登录；
   可以改为会话里只存 user_id，每次请求查一次短期缓存。
3. **AI 答疑尚未实现**：`ai-service/` 只有环境变量模板，建议等进度/成绩/权限数据稳定后再接 RAG。
4. **前端仍有模板残留**：`HelloWorld.vue`、`TheWelcome.vue`、`AboutView.vue` 等初始脚手架文件可删。
5. **`learningplatform.api` 已过期**：只有 5 个 user 接口，真实接口以各模块 `handler/routes.go` 为准。
