package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/exam/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// violationHandler POST /api/exam/:id/violation
//
// 前端在考试中检测到切屏/离开页面时调用一次，服务端累加计数。
// 为什么不只在前端判断：前端计数可以改，服务端计数才是老师信任的依据。
// 达到 tab_limit 后的处理（警告/强制交卷）留给老师端判断，服务端只如实记录。
func violationHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		count, err := logic.NewExamLogic(r.Context(), svcCtx).RecordViolation(examId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]int{"violations": count})
	}
}
