package user

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册用户模块的所有路由。
// 每个业务模块一个这样的文件，总路由（handler/routes.go）统一汇总。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			// 演示接口（验证前后端连通）
			{
				Method:  http.MethodGet,
				Path:    "/api/from/:name",
				Handler: HelloHandler(serverCtx),
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
}
