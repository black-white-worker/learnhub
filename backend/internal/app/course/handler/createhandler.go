package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/app/user"
	"learningplatform/internal/authz"
	"learningplatform/internal/svc"
)

// CreateHandler 课程创建接口：只有老师/管理员能创建（能力矩阵统一在 authz 包）。
// 创建者从登录态取，不信任前端传参，防止伪造"我是别人"。
func CreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireCapability(w, r, authz.CourseCreate) {
			return
		}
		u := user.CurrentUser(r)

		var req course.CreateRequest
		// 前端字段可能缺失（createdBy 由后端注入），解析失败也继续用零值
		_ = parseJSONBody(r, &req)
		req.CreatedBy = u.Username

		l := logic.NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}
