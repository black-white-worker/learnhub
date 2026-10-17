package user

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// RegisterHandler 注册接口的 HTTP 处理器：
// 解析请求体 → 调用注册业务逻辑 → 返回 JSON
func RegisterHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析请求体（JSON：username / password）
		var req RegisterRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 2. 调用注册业务逻辑
		l := NewRegisterLogic(r.Context(), svcCtx)
		resp, err := l.Register(&req)
		if err != nil {
			// 注册失败（参数为空 / 用户名已存在）
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			// 注册成功，返回用户名
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
