package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// SaveVideoProgressHandler 上报播放进度：POST /api/course/video/progress/:courseId/:videoId
//
// 进度按登录用户落库（改造前存在 Redis 且不分用户，全班共用一个进度条）。
// 用户 id 由服务端从会话解析，前端传什么都不采信。
func SaveVideoProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		var req course.SaveProgressRequest
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "请求参数格式错误"})
			return
		}
		req.CourseId = parseIntVar(r, "courseId")
		req.VideoId = parseIntVar(r, "videoId")
		req.UserId = cu.Id

		l := logic.NewVideoProgressLogic(r.Context(), svcCtx)
		resp, err := l.Save(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}

// GetVideoProgressHandler 读取上次播放进度：GET /api/course/video/progress/:courseId/:videoId
func GetVideoProgressHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		req := course.GetProgressRequest{
			CourseId: parseIntVar(r, "courseId"),
			VideoId:  parseIntVar(r, "videoId"),
			UserId:   cu.Id,
		}
		l := logic.NewVideoProgressLogic(r.Context(), svcCtx)
		resp, err := l.Get(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}
