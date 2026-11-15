package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/svc"
)

// ReprocessVideoHandler 重试失败的视频后处理：POST /api/course/video/reprocess/:courseId/:videoId
//
// 权限与上传一致（只有课程创建者/管理员）：转码吃服务器 CPU，
// 不该让随便哪个学生点一下就把机器占满。
func ReprocessVideoHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.ReprocessVideoRequest
		req.CourseId = parseIntVar(r, "courseId")
		req.VideoId = parseIntVar(r, "videoId")

		if !authorizeCourse(w, r, req.CourseId, authz.CourseUpload) {
			return
		}
		if req.VideoId <= 0 {
			courseError(r, w, course.ErrBadRequest("videoId 不合法"))
			return
		}

		l := logic.NewReprocessVideoLogic(r.Context(), svcCtx)
		resp, err := l.Reprocess(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}
