package handler

import (
	"context"
	"net"
	"net/http"
	"strings"

	"learningplatform/internal/app/user"
	"learningplatform/internal/app/user/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegisterHandler 注册接口的 HTTP 处理器：
// 解析请求体 → 提取客户端 IP（限流用）→ 调用注册业务逻辑 → 返回 JSON
func RegisterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析请求体（JSON：username / password）
		var req user.RegisterRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 2. 把客户端 IP 放入 context，供注册限流使用
		ctx := context.WithValue(r.Context(), user.ClientIPKey, resolveClientIP(r))

		// 3. 调用注册业务逻辑
		l := logic.NewRegisterLogic(ctx, svcCtx)
		resp, err := l.Register(&req)
		if err != nil {
			// 注册失败（参数为空 / 用户名已存在 / 触发限流）
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			// 注册成功，返回用户名
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// resolveClientIP 解析客户端真实 IP：
// 优先取 X-Forwarded-For（nginx 反代时带上），否则用 TCP 连接的 RemoteAddr。
func resolveClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能是 "ip1, ip2, ..."，取第一个（真实来源）
		if i := strings.Index(xff, ","); i > 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
