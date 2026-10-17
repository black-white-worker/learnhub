package handler

import (
	"learningplatform/internal/app/course"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterHandlers 汇总所有业务模块的路由注册。
// 每新增一个业务模块（course、ai...），在这里加一行即可。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	user.RegisterRoutes(server, serverCtx)
	course.RegisterRoutes(server, serverCtx)
	// 未来：ai.RegisterRoutes(server, serverCtx)
}
