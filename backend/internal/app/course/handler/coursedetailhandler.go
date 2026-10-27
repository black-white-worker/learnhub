package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// CourseDetailHandler 课程详情接口：GET /api/course/detail/:id
//
// 公开可读（首页/分享链接），但媒体地址按登录态与选课关系决定是否下发：
// 未登录返回 canWatch=false + 原因，视频 Url 为空。
func CourseDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.CourseDetailRequest
		if err := httpx.Parse(r, &req); err != nil {
			courseError(r, w, course.ErrBadRequest("参数错误"))
			return
		}

		// 未登录时 CurrentUser 返回 nil，由 logic 按"匿名"处理
		actor := user.CurrentUser(r)
		l := logic.NewCourseDetailLogic(r.Context(), svcCtx)
		resp, err := l.Detail(&req, actor)
		if err != nil {
			courseError(r, w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
