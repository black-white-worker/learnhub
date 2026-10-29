package handler

import (
	"net/http"

	"learningplatform/internal/app/user/logic"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// InfoHandler 当前用户信息接口：
// 登录态已由鉴权中间件校验，直接调业务逻辑返回用户信息（含角色与是否必须改密）。
func InfoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logic.NewInfoLogic(r.Context(), svcCtx)
		resp, err := l.Info()
		if err != nil {
			response.WriteError(w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
