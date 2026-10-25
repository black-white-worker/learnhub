package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// CreateHandler 课程创建接口的 HTTP 处理器：
// 只有老师和管理员能创建课程（问题2：鉴权已生效）
// 创建者直接从登录态取，不依赖前端传参（防伪造）。
func CreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u := user.CurrentUser(r)
		if u == nil || (u.Role != "teacher" && u.Role != "admin") {
			httpx.WriteJson(w, 403, map[string]string{"message": "只有老师或管理员才能创建课程"})
			return
		}

		var req course.CreateRequest
		if err := httpx.Parse(r, &req); err != nil {
			// createdBy 由后端注入，前端不传也不报错
			req = course.CreateRequest{
				Name:    req.Name,
				Intro:   req.Intro,
				Teacher: req.Teacher,
			}
		}
		req.CreatedBy = u.Username

		l := logic.NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
