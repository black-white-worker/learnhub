package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// VideoPlayHandler 视频播放信息接口：GET /api/course/video/play/:courseId/:videoId
//
// 需登录 + 课程媒体权限：返回的 Url 是 30 分钟预签名链接，
// 只签发给有观看权限的人（见 course/media_access.go）。
func VideoPlayHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.PlayVideoRequest
		if err := httpx.Parse(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("参数错误"))
			return
		}
		if !guardCourseMedia(w, r, svcCtx, req.CourseId) {
			return
		}

		l := logic.NewVideoPlayLogic(r.Context(), svcCtx)
		resp, err := l.Play(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
