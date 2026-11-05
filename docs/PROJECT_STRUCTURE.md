# 项目结构说明（逐文件注释）

> 学习平台单体应用：Go（go-zero）后端 + Vue3 前端 + 独立 AI 服务（空壳）。
> 后端按业务域分包，每个接口成对出现 `xxxhandler.go`（HTTP 入口）/ `xxxlogic.go`（业务逻辑），并配 `_test.go` 单测。

---

## 仓库根目录

```
learningplatform/
├─ .env                          # 运行环境变量（DB/Redis/MinIO/JWT 等），已被 gitignore
├─ .env.example                  # 环境变量模板，提交到仓库供参考
├─ docker-compose.yml            # 容器编排：backend/frontend/mysql/redis/minio/nginx
├─ README.md                     # 项目总说明
├─ .vscode/launch.json           # VS Code 调试配置（Go Delve + 多容器启动）
├─ ai-service/                   # AI 服务（尚未实现，仅契约文档）
│  ├─ .env / .env.example        # AI 服务环境变量（含 LLM Key 模板）
│  └─ README.md                  # AI 功能契约草案（X-Internal-Token 校验、内网暴露）
├─ docs/
│  ├─ architecture.md            # 后端架构说明
│  ├─ microservices.md           # 微服务拆分规划
│  ├─ performance.md             # 性能设计说明
│  └─ PROJECT_STRUCTURE.md       # 本文件
├─ scripts/                      # 运维脚本（PowerShell）
│  ├─ start.ps1                  # 一键启动/构建（含 -ForceBuild 逻辑，本次已修）
│  ├─ start-backend.ps1          # 仅启动后端
│  ├─ rebuild-app.ps1            # 重建应用镜像
│  ├─ e2e-step1.ps1              # 端到端测试第 1 步（环境准备）
│  ├─ e2e-step2-exam.ps1         # 端到端测试第 2 步（考试流程）
│  └─ perf/seed_gradebook_bench.sql  # 成绩册压测种子数据
├─ load-test/                    # Go 压测脚本
│  ├─ main.go                    # 并发压测入口
│  └─ go.mod
├─ load-test-report/             # 压测报告
│  ├─ REPORT.md                  # 综合报告
│  └─ AB_REPORT_2026-10-28.md    # AB 压测明细
└─ frontend/  backend/           # 见下方独立章节
```

---

## backend/（Go 后端）

```
backend/
├─ go.mod / go.sum              # 依赖：go-zero、sqlx、mysql、redis、minio、bcrypt 等
├─ Dockerfile                   # 多阶段构建：builder(Go 编译) → stage-2(alpine + LibreOffice + 中文字体)
├─ .dockerignore / .gitignore   # 构建/提交忽略清单
├─ learningplatform.exe         # 本机编译产物（Windows，未进 git）
├─ cmd/api/main.go              # 启动类：加载配置→建 ServiceContext→注册路由→启后台任务→优雅退出
├─ etc/learningplatform-api.yaml# 运行配置（端口/DB/Redis/MinIO/超时等），已被 gitignore 部分字段
└─ internal/                    # 业务代码（见下）
```

### backend/internal/

```
internal/
├─ config/config.go             # 配置结构体，映射 yaml（DB/Redis/MinIO/JWT/限流等）
├─ svc/servicecontext.go        # 依赖注入容器：DB/Redis/Oss/ClassStore/ExamStore 等单例
├─ handler/
│  ├─ routes.go                 # 全局路由注册（汇总各业务域）
│  └─ health.go                 # 健康检查接口 /healthz
├─ platform/                    # 横切基础设施
│  ├─ middleware/
│  │  ├─ bodylimit.go           # 请求体大小限制（全局 1GB，按路由降 1MB/上传放宽）
│  │  └─ requestid.go           # 请求追踪 ID（注入日志）
│  ├─ oss/oss.go                # 对象存储封装（MinIO：私有桶+预签名）
│  ├─ quota/quota.go            # 上传配额（按用户每日上限）
│  ├─ response/response.go      # 统一响应体封装
│  └─ concurrency/concurrency.go # 并发工具（限流/信号量）
├─ domain/                      # 纯领域规则（无 DB 依赖，独立单测）
│  ├─ authz/authz.go            # 鉴权规则（课程归属/角色判断）
│  ├─ examrules/examrules.go    # 考试规则（时间窗/防重复交卷/答案下发时机）
│  └─ gradebook/gradebook.go    # 成绩册计算规则
├─ model/                       # 数据访问层（sqlx，参数化查询）
│  ├─ class.go                  # 班级/成员/签到/公告/讨论/作业
│  ├─ course.go                 # 课程/资源/视频元数据
│  ├─ exam.go                   # 考试/题目/提交/答卷（含 SubmitOnceTx 事务方法）
│  ├─ grade.go                  # 成绩项/成绩行（含 SetGradeScoreTx 事务方法）
│  ├─ grade_queue.go            # 成绩异步队列
│  ├─ grade_row.go              # 成绩行模型
│  ├─ mywork.go                 # 我的作业聚合
│  ├─ upload_session.go         # 分片上传会话
│  ├─ user.go                   # 用户表（bcrypt 密码）
│  ├─ video_progress.go         # 视频观看进度
│  └─ query_integration_test.go # 模型层集成测试
├─ app/                         # 业务域（handler+logic+types+路由）
│  ├─ user/                     # 用户与鉴权
│  ├─ course/                   # 课程与资源/视频
│  ├─ exam/                     # 考试与阅卷
│  └─ class/                    # 班级与教学互动
```

### app/user/（用户与鉴权）

```
app/user/
├─ authmiddleware.go            # JWT/会话中间件：解析 token→注入 CurrentUser
├─ ctxkeys.go                   # 上下文键定义
├─ errors.go                    # 用户域错误定义
├─ types.go                     # 请求/响应结构体
├─ handler/
│  ├─ routes.go                 # 用户域路由
│  ├─ loginhandler.go           # 登录入口
│  ├─ registerhandler.go        # 注册入口
│  ├─ logouthandler.go          # 登出（清 Redis 会话）
│  ├─ infohandler.go            # 获取当前用户信息
│  └─ passwordhandler.go        # 修改密码
└─ logic/
   ├─ loginlogic.go             # 登录逻辑（bcrypt 校验/升级 + Redis 会话）
   ├─ login_guard.go            # 登录失败锁（5 次锁 10 分钟）
   ├─ registerlogic.go          # 注册逻辑
   ├─ logoutlogic.go            # 登出逻辑
   ├─ infologic.go              # 信息逻辑
   └─ passwordlogic.go          # 改密逻辑
```

### app/course/（课程与资源/视频）

```
app/course/
├─ types.go                     # 课程域结构体
├─ authz.go                     # 课程操作鉴权（所有者/教师）
├─ cache.go                     # 课程缓存封装
├─ media.go / media_access.go   # 媒体访问鉴权（视频/资源是否可看）
├─ ffmpeg.go                    # 视频转码封装（调用 ffmpeg）
├─ thumbnail.go                 # 缩略图生成
├─ uploadcommon.go              # 上传公共逻辑（分片/key 拼装）
├─ objectrefs.go                # 对象引用记录（用于孤儿清理）
├─ model.go                     # 课程域模型辅助
├─ mysql_store.go               # 课程域 MySQL 操作
├─ orphansweeper.go             # 后台孤儿对象清理（本次已加 context 优雅退出）
├─ handler/
│  ├─ routes.go / guard.go      # 路由 + 上传前鉴权守卫
│  ├─ createhandler.go / deletecoursehandler.go / editcoursehandler.go / listhandler.go / coursedetailhandler.go
│  ├─ coveruploadhandler.go     # 课程封面上传
│  ├─ videoplayhandler.go       # 视频播放（预签名/鉴权）
│  ├─ videouploadhandler.go     # 视频上传
│  ├─ videoprogresshandler.go   # 观看进度上报
│  ├─ resourceuploadhandler.go / resourcelisthandler.go / resourcedeletehandler.go / resourcepreviewhandler.go
│  └─ multipartuploadhandler.go # 分片上传
└─ logic/
   ├─ createlogic.go / editcourselogic.go / deletecourselogic.go / listlogic.go / coursedetaillogic.go
   ├─ coveruploadlogic.go / videouploadlogic.go / videoplaylogic.go / videoprogresslogic.go
   ├─ resourceuploadlogic.go / resourcelistlogic.go / resourcedeletelogic.go
   └─ multipartuploadlogic.go   # 分片上传合并逻辑
```
> 每个 handler/logic 均配套 `_test.go`（含 fakeoss、flight、watch 等测试辅助）。

### app/exam/（考试与阅卷）

```
app/exam/
├─ types.go                     # 考试域结构体
├─ handler/
│  ├─ routes.go                 # 考试域路由
│  └─ violationhandler.go       # 切屏/违规上报接口
└─ logic/
   ├─ examlogic.go              # 核心：考试详情/交卷/批改（本次交卷+成绩册已包事务）
   ├─ autosubmit.go             # 超时自动收卷（本次已加 context 优雅退出）
   ├─ score.go                  # 客观题自动评分
   └─ *_test.go                 # 评分/规范化/提交评分/自动收卷集成测试
```

### app/class/（班级与教学互动）

```
app/class/
├─ types.go                     # 班级域结构体
├─ common.go / log.go           # 公共工具/日志
├─ examphase.go                 # 考试阶段状态机（整场开始/结束/答案下发）
├─ assignment_utils.go          # 作业聚合工具
├─ bizerror_test.go / routes_test.go
├─ handler/                     # 约 50 个接口，命名即功能：
│  ├─ createclasshandler.go     # 建班
│  ├─ joinclasshandler.go / applyteacherhandler.go / removememberhandler.go / listmembershandler.go / myclasseshandler.go
│  ├─ bindcoursehandler.go / unbindcoursehandler.go   # 班级绑定/解绑课程
│  ├─ createannouncementhandler.go / listannouncementhandler.go / deleteannouncementhandler.go / markannouncementreadhandler.go
│  ├─ creatediscussionhandler.go / listdiscussionshandler.go / discussiondetailhandler.go / deletediscussionhandler.go / replydiscussionhandler.go
│  ├─ startsigninhandler.go / dosigninhandler.go / endsigninhandler.go / getactivesigninhandler.go / listsigninrecordshandler.go / listsigninshistoryhandler.go
│  ├─ createassignmenthandler.go / listassignmentshandler.go / assignmentdetailhandler.go / assignmenteditorhandler.go / submitassignmenthandler.go / uploadassignmenthandler.go / listsubmissionshandler.go / gradesubmissionhandler.go
│  ├─ gradebookhandlers.go / listgradeshandler.go   # 成绩册
│  ├─ notificationhandlers.go / listallusershandler.go / updateuserrolehandler.go / adminuserhandlers.go / allclasseshandler.go
│  └─ routes.go                 # 班级域路由汇总
└─ logic/                       # 与 handler 一一对应的业务逻辑（同名 *_logic.go）
```

---

## frontend/（Vue3 前端）

```
frontend/
├─ package.json / package-lock.json   # 依赖：Vue3 / Vue Router / Pinia / Element Plus / Axios
├─ vite.config.js               # Vite 构建配置（含代理 /api → backend）
├─ index.html                   # 应用入口 HTML
├─ jsconfig.json                # JS 路径别名配置
├─ nginx.conf                   # 生产 Nginx（同源代理 /api，无跨域问题）
├─ Dockerfile                   # 前端镜像（build → nginx 托管）
├─ .dockerignore / .gitignore   # 忽略清单
├─ dist/                        # 构建产物（本次评语修复需重新构建）
├─ public/                      # 静态资源（favicon 等）
├─ README.md                    # 前端说明
├─ tests/                       # 前端测试
└─ src/
   ├─ main.js                   # 应用引导（挂载 App + 路由 + Pinia）
   ├─ App.vue                   # 根组件（布局/全局样式）
   ├─ api/
   │  ├─ client.js              # Axios 实例（拦截器/错误处理）
   │  └─ index.js               # 各业务接口封装（user/exam/course/class）
   ├─ router/
   │  ├─ index.js               # 路由表
   │  └─ guard.js               # 路由守卫（登录态/角色校验）
   ├─ utils/
   │  └─ courseCreateFlow.js    # 课程创建流程工具
   ├─ assets/                   # 图片/样式等静态资源
   ├─ components/
   │  ├─ CourseResourcePanel.vue  # 课程资源面板
   │  ├─ QuestionEditor.vue       # 题目编辑器（出题用）
   │  └─ class/
   │     ├─ AssignmentTab.vue     # 班级-作业标签页
   │     ├─ DiscussTab.vue        # 班级-讨论标签页
   │     ├─ GradeTab.vue          # 班级-成绩册标签页
   │     ├─ NoticeTab.vue         # 班级-公告标签页
   │     └─ SigninTab.vue         # 班级-签到标签页
   └─ views/                    # 页面级组件（路由对应）
      ├─ HomeView.vue            # 首页
      ├─ LoginView.vue / RegisterView.vue   # 登录/注册
      ├─ ProfileView.vue         # 个人资料
      ├─ NotFoundView.vue        # 404
      ├─ CourseListView.vue / CourseDetailView.vue / CourseCreateView.vue   # 课程列表/详情/创建
      ├─ CourseWatchView.vue     # 课程视频观看页
      ├─ CourseResourcesView.vue # 课程资源页
      ├─ ClassListView.vue / ClassDetailView.vue   # 班级列表/详情
      ├─ ExamListView.vue / ExamDetailView.vue     # 考试列表/详情（★评语显示、切屏检测在此）
      ├─ AssignmentDetailView.vue # 作业详情
      ├─ SpeedGraderView.vue     # 老师批改端（★保存评语接口）
      └─ admin/
         ├─ AdminUsersView.vue   # 后台-用户管理
         └─ AdminCoursesView.vue # 后台-课程管理
```
