package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/authz"
	"learningplatform/internal/svc"
)

// ResourceDeleteHandler 课程资源删除接口：DELETE /api/course/resource/:courseId/:resourceId
// 只有课程创建者/管理员可以删。
func ResourceDeleteHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.DeleteResourceRequest
		req.CourseId = parseIntVar(r, "courseId")
		req.ResourceId = parseIntVar(r, "resourceId")

		if !authorizeCourse(w, r, req.CourseId, authz.CourseResourceDelete) {
			return
		}

		l := logic.NewResourceDeleteLogic(r.Context(), svcCtx)
		resp, err := l.Delete(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}
