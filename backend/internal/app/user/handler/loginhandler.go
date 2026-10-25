package handler

import (
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/app/user/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// LoginHandler 登录接口的 HTTP 处理器：
// 解析请求体 → 调用业务逻辑 → 返回 JSON
func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析请求体（JSON：username / password）
		var req user.LoginRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 2. 调用登录业务逻辑
		l := logic.NewLoginLogic(r.Context(), svcCtx)
		resp, err := l.Login(&req)
		if err != nil {
			// 登录失败（用户名或密码错误）
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			// 登录成功，返回 token
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
