package handler

import (
	"context"
	"net"
	"net/http"
	"strings"

	"learningplatform/internal/app/user"
	"learningplatform/internal/app/user/logic"
	"learningplatform/internal/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// LoginHandler 登录接口：
// 解析请求体 → 注入客户端 IP（用于登录限流）→ 调用业务逻辑 → 返回 JSON。
//
// 错误统一走 response.WriteError，所以 401（密码错）/ 429（被限流）
// 都能按真实语义返回，而不是统统变成 500。
func LoginHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req user.LoginRequest
		if err := logic.ParseLoginJSON(r, &req); err != nil {
			response.WriteError(w, &user.AuthError{Code: http.StatusBadRequest, Msg: "用户名和密码不能为空"})
			return
		}

		ctx := context.WithValue(r.Context(), user.ClientIPKey, ClientIP(r))
		l := logic.NewLoginLogic(ctx, svcCtx)
		resp, err := l.Login(&req)
		if err != nil {
			// 401（密码错）/ 429（被限流）按真实语义返回
			response.WriteError(w, err)
			return
		}
		// 注意：登录成功仍返回"裸 data"结构（{token,username,role}），
		// 前端 LoginView 直接读 r.data.token；这里不要套 {code,message,data} 信封，
		// 否则会导致"登录成功但拿不到 token → 后续全部 401"。
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// ClientIP 取客户端 IP：优先反向代理头，其次 RemoteAddr。
// 只用于限流计数，不做安全断言（X-Forwarded-For 可伪造，但配合用户名已经足够）。
func ClientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		if i := strings.Index(xf, ","); i >= 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
