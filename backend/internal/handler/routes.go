package handler

import (
	classhandler "learningplatform/internal/app/class/handler"
	coursehandler "learningplatform/internal/app/course/handler"
	examhandler "learningplatform/internal/app/exam/handler"
	notificationhandler "learningplatform/internal/app/notification/handler"
	userhandler "learningplatform/internal/app/user/handler"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterHandlers 汇总所有业务模块的路由注册。
// 每新增一个业务模块（course、notification、ai...），在这里加一行即可。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	userhandler.RegisterRoutes(server, serverCtx)
	coursehandler.RegisterRoutes(server, serverCtx)
	classhandler.RegisterRoutes(server, serverCtx)
	examhandler.RegisterRoutes(server, serverCtx)
	// 通知中心：偏好 REST 接口 + WebSocket 实时推送路由
	notificationhandler.RegisterRoutes(server, serverCtx)
}
