package course

import (
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// CreateHandler 课程创建接口的 HTTP 处理器：
// 只有老师和管理员能创建课程
func CreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 权限校验：只有老师/管理员能创建课程
		u := user.CurrentUser(r)
		if u == nil || (u.Role != "teacher" && u.Role != "admin") {
			httpx.WriteJson(w, 403, map[string]string{"message": "只有老师或管理员才能创建课程"})
			return
		}

		var req CreateRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
