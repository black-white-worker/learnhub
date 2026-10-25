package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// EditCourseHandler 课程编辑接口的 HTTP 处理器：
// 解析 JSON 请求体（id / name / intro / teacher）→ 调用业务逻辑 → 返回 JSON。
//
// 前端调用示例：
//
//	PUT /api/course/edit
//	{ "id": 1, "name": "概率论", "intro": "...", "teacher": "..." }
func EditCourseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.EditCourseRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewEditCourseLogic(r.Context(), svcCtx)
		resp, err := l.EditCourse(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
