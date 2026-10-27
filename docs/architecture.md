# 架构说明

## 分层

```
HTTP 请求
   │
   ▼
internal/app/<context>/handler     解析参数、鉴权守卫、写响应
   │
   ▼
internal/app/<context>/logic       用例编排：取数 → 规则 → 落库
   │                    │
   │                    └──────────► internal/domain/*      纯规则（无 IO）
   ▼
internal/model                     MySQL 访问（手写 SQL）
   │
   ▼
internal/platform/*                技术设施：oss / quota / middleware / response
```

依赖方向是单向的：

- `domain` 不 import 任何业务包、数据库或 HTTP 库；
- `platform` 只依赖标准库与第三方基础设施客户端；
- `app` 可以依赖 `domain` / `platform` / `model`；
- `model` 只依赖 `sqlx`。

这条规则让 `domain` 里的规则可以原样搬到新服务，也让单测不需要起数据库。

## 一次请求的生命周期

以「学生交卷」为例：

1. `middleware.RequestIDMiddleware` 注入 `X-Request-ID`；
2. `middleware.BodyLimit` 把请求体限制在 1MB；
3. `user.AuthMiddleware` 用 `Authorization: Bearer <token>` 查 Redis 会话，
   并顺便刷新一次角色短缓存（管理员改角色后 60 秒内生效）；
4. `exam/handler` 解析路径与 JSON；
5. `ExamLogic.Submit`：
   - `class.RequireClassMember` 确认是班内成员；
   - `examrules.CheckSubmit` 用服务端时间判定是否超时；
   - `ScoreExam`（纯函数）算客观题得分；
   - `ExamStore.SubmitOnce` 条件更新（`WHERE status='started'`）保证只交一次；
   - `recordExamScore` 写成绩册（失败只记日志，不回滚答卷）；
6. 响应仍沿用"裸 data"结构，错误走 `{code, message}`。

## 关键决策与取舍

### 1. 为什么保留"裸 data"响应？

历史接口和 19 个前端页面都按 `res.data.xxx` 取值。一次性改成 `{code,message,data}`
会牵动所有页面，收益只是形式统一。这里的取舍是：**错误统一信封、成功保持兼容**，
新接口可以自由选择，前端用 `client`（解包）或 `rawClient`（不解包）。

### 2. 业务错误用 `StatusCode()` 鸭子类型

`platform/response` 不 import 任何业务包，只识别实现了 `StatusCode() int` 的 error。
业务包定义自己的 `BizError` / `Denied` / `Quota.Exceeded` 即可输出正确的 HTTP 状态码，
不会出现"课程不存在 → 500"。

### 3. 媒体鉴权放在 handler 的守卫里

课程模块的 logic 方法签名是"只接收请求体"（历史原因，测试也按这个签名写）。
媒体守卫 `guardCourseMedia` 因此放在 HTTP 边界，统一处理"未登录 401 / 无权限 403"。
`ResolveMediaAccess` 本身是纯函数 + 一个取数函数，规则仍可单测。

### 4. 对象生命周期以"引用集合"为准

MinIO 里的对象没有事务保护，删错就没了。所以：

- 引用集合集中在 `course/objectrefs.go`，新增对象类型只改这一处；
- 清理器只处理白名单前缀（`cover/`、`videos/`、`resources/`、`previews/`），
  未知前缀一律不碰；
- 清理前有 10 分钟安全窗口，避免误删"刚上传、引用还没写回"的对象。

长期方向是 Moodle 的 `mdl_files` 模型：对象表作为唯一事实来源，按引用计数回收。

### 5. 考试时间主权在服务端

前端倒计时只是展示，`deadlineAt` 由服务端计算；交卷、草稿、自动收卷都走同一套
`examrules`。这样改本地时间、关页面、断网都不会绕过规则。

超时收卷按最后一份草稿判分：草稿由前端每 15 秒上报，服务端只覆盖 `answers` 字段，
不改状态；收卷时再统一算分并写成绩册。

### 6. 请求体按路径分级限长

`MaxBytes` 是全局配置，为了视频上传必须放到 1GB。真正的防线是中间件：
默认 1MB，只有 `/api/course/upload/` 和 `/api/assignment/upload` 放宽。
业务层的 `ParseJSON` 只需要把 `http.MaxBytesError` 翻译成 413。

## 数据模型（核心表）

| 表 | 作用 |
|---|---|
| `users` / `class_members` | 账号、全局角色、班内角色 |
| `courses` / `course_videos` / `course_resources` | 课程与媒体元信息（对象路径 + URL） |
| `classes` / `class_courses` | 班级与课程绑定（决定媒体访问范围） |
| `assignments` / `assignment_submissions` / `assignment_questions` | 作业 |
| `exams` / `exam_questions` / `exam_submissions` | 考试与答卷状态机 |
| `grade_items` / `grade_scores` | 成绩册（每个作业/考试一条 item） |
| `video_progress` | 按用户的学习进度 |
| `announcements` / `discussion*` / `class_signin*` / `notifications` | 班级互动 |

迁移由 Flyway 管理（`backend/deploy/sql/migration`），每次结构变更新增一个 `V<n>__*.sql`，
不要修改已执行的文件（校验和会变）。

## 扩展点

- **新增业务模块**：复制 `internal/app/<context>` 的 handler/logic 结构，
  在 `internal/handler/routes.go` 注册一行；纯规则放 `internal/domain`。
- **AI 服务**：`ai-service/` 预留；主服务通过 HTTP 调用，不直接读写 AI 的向量库。
- **观测**：`/readyz` 已聚合 MySQL/Redis；下一步可接 go-zero 的 Prometheus exporter。
