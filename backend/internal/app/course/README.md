# course 课程模块（占位）

未来在这里放课程相关的 handler / logic / 类型定义，例如：

- 课程列表 `GET /api/course/list`
- 课程详情 `GET /api/course/:id`
- 课程创建 `POST /api/course`（教师）

每个业务模块一个 `RegisterRoutes(server, svcCtx)` 入口，在 `internal/handler/routes.go` 汇总注册。
