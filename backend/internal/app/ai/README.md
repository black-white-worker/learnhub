# ai AI 模块（占位）

未来在这里放 AI 相关功能（对接 Python + LangChain/LangGraph 的服务）：

- AI 答疑接口（通过 gRPC/SSE 转发到 Python AI 服务）
- 智能推荐

每个业务模块一个 `RegisterRoutes(server, svcCtx)` 入口，在 `internal/handler/routes.go` 汇总注册。
