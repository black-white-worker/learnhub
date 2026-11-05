package handler

import (
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册用户模块的所有路由。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			// 演示接口（验证前后端连通）
			{
				Method:  http.MethodGet,
				Path:    "/api/from/:name",
				Handler: user.HelloHandler(serverCtx),
			},
			// 登录接口
			{
				Method:  http.MethodPost,
				Path:    "/api/user/login",
				Handler: LoginHandler(serverCtx),
			},
			// 注册接口
			{
				Method:  http.MethodPost,
				Path:    "/api/user/register",
				Handler: RegisterHandler(serverCtx),
			},
		},
	)

	// 需要登录的接口：统一挂鉴权中间件（Redis 会话校验）
	server.AddRoutes(
		rest.WithMiddleware(
			user.NewAuthMiddleware(serverCtx).Handle,
			[]rest.Route{
				{
					Method:  http.MethodPost,
					Path:    "/api/user/logout",
					Handler: LogoutHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/api/user/info",
					Handler: InfoHandler(serverCtx),
				},
				{
					Method:  http.MethodPost,
					Path:    "/api/user/password",
					Handler: PasswordHandler(serverCtx),
				},
			}...,
		),
	)
}
