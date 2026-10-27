package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/authz"
	"learningplatform/internal/svc"
)

// EditCourseHandler 课程编辑接口：只有课程创建者（或管理员）能改。
// PUT /api/course/edit  { "id": 1, "name": "...", "intro": "...", "teacher": "..." }
func EditCourseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.EditCourseRequest
		if err := parseJSONBody(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("请求参数格式错误"))
			return
		}
		if !authorizeCourse(w, r, req.Id, authz.CourseEdit) {
			return
		}

		l := logic.NewEditCourseLogic(r.Context(), svcCtx)
		resp, err := l.EditCourse(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}
