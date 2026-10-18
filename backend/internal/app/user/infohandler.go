package user

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// InfoHandler 当前用户信息接口的 HTTP 处理器：
// 登录态已由鉴权中间件校验，直接调业务逻辑返回用户信息
func InfoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := NewInfoLogic(r.Context(), svcCtx)
		resp, err := l.Info()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
