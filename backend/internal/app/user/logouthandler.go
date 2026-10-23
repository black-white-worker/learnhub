package user

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// LogoutHandler 登出接口的 HTTP 处理器：
// token 已由鉴权中间件校验并放入 context，这里直接取出来调业务逻辑
func LogoutHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 从 context 取出 token（鉴权中间件放进去的）
		token, _ := r.Context().Value(authTokenKey).(string)

		// 2. 调用登出业务逻辑（删除 Redis 会话）
		l := NewLogoutLogic(r.Context(), svcCtx)
		resp, err := l.Logout(token)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
