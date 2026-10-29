# 微服务拆分方案

当前是**模块化单体**：一个进程、一个数据库，但业务边界已经按服务切好。
目标是按"变更频率 + 数据归属"拆，而不是为了拆而拆。

## 目标服务

| 服务 | 职责 | 数据归属 | 拆分优先级 |
|---|---|---|---|
| `user-svc` | 账号、登录、会话、角色、密码 | `users` | 低（认证是所有服务的依赖，先稳定） |
| `course-svc` | 课程、媒体、上传、学习进度 | `courses` `course_videos` `course_resources` `video_progress` | 中 |
| `class-svc` | 班级、成员、公告、讨论、签到、作业 | `classes` `class_members` 等 | 中 |
| `exam-svc` | 考试、答卷、判分 | `exams` `exam_questions` `exam_submissions` | 高（写入峰值独立，便于限流/扩容） |
| `ai-svc` | 答疑、作业初评、向量检索 | 独立向量库 | 下一步先落地 |

## 边界原则

1. **每个服务只写自己的表**。跨服务读取走 API 或事件，不直连对方数据库。
2. **纯规则随业务走**。`domain/authz`、`domain/gradebook`、`domain/examrules`
   可以被多个服务复用（复制或发成内部 Go module），不依赖数据库。
3. **共享技术设施下沉**。`platform/oss`、`platform/quota`、`platform/response`
   抽成内部库；每个服务保留自己的 `cmd/<name>`。
4. **认证集中**。token 校验由网关或 user-svc 提供，业务服务只信任已解析的 actor。

## 拆分顺序（建议）

### 第 1 步：exam-svc（低风险、收益明确）

- 数据：`exams`、`exam_questions`、`exam_submissions` 三张表迁出；
- 依赖：需要 `class_members` 判断班内角色 → 通过 class-svc 的
  `GET /internal/members/{classId}/{userId}` 获取，结果缓存 60 秒；
- 写路径：交卷、草稿、收卷全部在 exam-svc 内完成；
- 成绩册：exam-svc 通过事件（或直接调用）通知 `class-svc` 更新 `grade_scores`。

> 这一步最容易做，因为考试是"写多读少、峰值明显"的模块，
> 独立后可以单独限流、单独扩容，且失败不影响课程浏览。

### 第 2 步：course-svc（媒体边界清晰）

- 课程、视频、讲义、上传、进度迁出；
- 媒体鉴权需要"课程绑定班级 + 用户是否在班"：改为调用 class-svc 的
  `GET /internal/courses/{courseId}/enrollment?userId=`；
- 预签名逻辑留在 course-svc（只有它持有 MinIO 凭证）。

### 第 3 步：class-svc（班级协作）

- 班级、公告、作业、讨论、签到、消息迁出；
- 作业与成绩册耦合较紧，建议与 `grade_items/grade_scores` 一起迁；
- 通知可以继续用站内消息表，后续再抽 notification-svc。

### 第 4 步：AI（新建，不拆）

`ai-service` 从一开始就是独立进程：

```
POST /answer      { classId, courseId, question }  → { answer, refs[] }
POST /grade       { assignmentId, submission }     → { score, comment, confidence }
POST /embed       { courseId, resourceId }         → { ok }
```

- 主服务只通过 HTTP 调用，不把向量库放进业务库；
- 首次落地建议只做"答疑 RAG"：讲义/公告切片 → embedding → 检索 → LLM；
- 需要回调主服务时用内部 token + 幂等键，避免重试放大。

## 拆分时要注意的坑

| 问题 | 现在的状态 | 拆分后要做什么 |
|---|---|---|
| 跨表 JOIN | 成绩册 `ListGradebookRaw` 直接 JOIN 班级/考试 | 改为各服务提供聚合接口，或在独立读模型里物化 |
| 事务 | 依赖单库事务（如建班 + 加成员） | 改成 saga / 补偿；建班失败要能回滚 |
| 身份 | 每次请求查 Redis + 角色短缓存 | 网关统一校验，向下游透传 actor |
| 对象清理 | 扫全表反推引用 | 每个服务维护自己的对象表或接收删除事件 |
| 配置 | 一个 yaml | 每个服务一份，共享的走环境变量/配置中心 |
| 迁移 | 一个 Flyway 目录 | 每个服务一个 migration 目录与独立 schema |

## 现在就该做的准备（不改架构）

1. 业务包不直接 import 其他业务包的 internals 之外的东西（目前只共享 `class.CtxUser` 等类型）；
2. 新增跨模块调用时，先定义接口再实现，避免直接依赖具体 store；
3. 所有对外错误保持 `{code,message}`，拆分后网关可以直接透传；
4. 关键写操作保持幂等（如 `SubmitOnce`、`SaveDraft`、成绩写入的 upsert）。
