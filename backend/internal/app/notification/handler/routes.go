package handler

import (
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册通知模块的全部路由。
// 偏好接口需登录（走 AuthMiddleware）；WS 接口单独注册（token 走 query 参数）。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	prefRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/api/notification-prefs", Handler: listPrefsHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/notification-prefs", Handler: updatePrefsHandler(serverCtx)},
	}
	server.AddRoutes(rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, prefRoutes...))

	// WebSocket 长连接：浏览器无法在握手时带自定义请求头，token 放在 query 参数里。
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/ws/notifications", Handler: wsNotificationsHandler(serverCtx)},
	})
}
