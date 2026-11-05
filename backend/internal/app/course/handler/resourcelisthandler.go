package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ResourceListHandler 课程资源列表接口：GET /api/course/resources/:courseId
//
// 需登录 + 课程媒体权限：列表里的每个 URL 都是预签名的，
// 放在公开路由上等于把讲义无条件放出去（历史问题）。
func ResourceListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.ListResourcesRequest
		if err := httpx.Parse(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("参数错误"))
			return
		}
		if !guardCourseMedia(w, r, svcCtx, req.CourseId) {
			return
		}

		l := logic.NewResourceListLogic(r.Context(), svcCtx)
		resp, err := l.List(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
