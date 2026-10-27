package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/app/user/logic"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// PasswordHandler 修改密码：POST /api/user/password（需登录）。
func PasswordHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req user.ChangePasswordRequest
		if err := decodeJSON(r, &req); err != nil {
			// 请求体大小已由 BodyLimit 中间件兜底；这里只可能是格式问题
			response.WriteError(w, user.NewAuthError(http.StatusBadRequest, "请求格式不正确"))
			return
		}
		resp, err := logic.NewPasswordLogic(r.Context(), svcCtx).ChangePassword(&req)
		if err != nil {
			response.WriteError(w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// decodeJSON 读取并解析 JSON 请求体。
func decodeJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return io.EOF
	}
	return json.Unmarshal(body, v)
}
