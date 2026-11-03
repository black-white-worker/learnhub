package handler

import (
	"encoding/json"
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/platform/ws"
	"learningplatform/internal/svc"

	"github.com/gorilla/websocket"
)

// upgrader 将 HTTP 连接升级为 WebSocket。
// CheckOrigin 开发期放开，生产环境应改为校验你的前端域名。
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// wsNotificationsHandler GET /ws/notifications?token=xxx
// 浏览器建立 WebSocket 时无法自定义请求头，因此 token 走查询参数；
// 服务端用与 AuthMiddleware 相同的 Redis key（user.SessionKeyPrefix + token）校验会话。
func wsNotificationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		val, err := svcCtx.Redis.Get(user.SessionKeyPrefix + token)
		if err != nil || val == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var session struct {
			Id       int64  `json:"id"`
			Username string `json:"username"`
		}
		if err := json.Unmarshal([]byte(val), &session); err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// 与 class.CurrentUser 保持一致：滚动发布前写入的老会话没有 id，
		// 回落到用户名查一次 id，避免"REST 能登录、WebSocket 一直 401"。
		uid := session.Id
		if uid <= 0 && session.Username != "" {
			if id, e := svcCtx.UserStore.GetIdByUsername(session.Username); e == nil {
				uid = id
			}
		}
		if uid <= 0 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return // 升级失败，响应已由 upgrader 写出
		}

		client := &ws.Client{
			Hub:    svcCtx.NotifHub,
			UserID: uid,
			Conn:   conn,
			Send:   make(chan []byte, 256),
		}
		svcCtx.NotifHub.Register(client)
		go client.WritePump() // 写：独立 goroutine
		client.ReadPump()     // 读：当前 goroutine（退出即代表连接关闭）
	}
}
