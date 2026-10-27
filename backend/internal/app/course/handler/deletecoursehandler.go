package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/authz"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

// DeleteCourseHandler 课程删除接口：只有课程创建者（或管理员）能删。
// 改造前只挂了登录中间件——任何学生都能删掉别人的课。
func DeleteCourseHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req course.DeleteCourseRequest
		req.Id = parseIntVar(r, "id")

		if !authorizeCourse(w, r, req.Id, authz.CourseDelete) {
			return
		}

		l := logic.NewDeleteCourseLogic(r.Context(), svcCtx)
		resp, err := l.Delete(&req)
		if err != nil {
			courseError(r, w, err)
			return
		}
		courseOK(r, w, resp)
	}
}

// parseIntVar 取路径参数里的整数，取不到返回 0。
func parseIntVar(r *http.Request, key string) int {
	if vars := pathvar.Vars(r); vars != nil {
		if s := vars[key]; s != "" {
			n := 0
			for _, ch := range s {
				if ch < '0' || ch > '9' {
					return 0
				}
				n = n*10 + int(ch-'0')
			}
			return n
		}
	}
	return 0
}
