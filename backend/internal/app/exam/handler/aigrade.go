package handler

// 考试主观题 AI 初评的 HTTP 入口。与作业侧（class/handler/aigradehandler.go）
// 同一套约定：载荷由 logic 从库里组装，请求体没有业务字段；
// worker 回调复用作业的回调入口（按建议表 assignment_id 分发），这里只注册老师端两个接口。
import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/exam/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// examAiGradeTriggerHandler POST /api/exam/submission/:sid/ai-grade
// 老师对一份考试提交点「AI 初评」。
func examAiGradeTriggerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		sid := class.ParseIntPath(r, "sid")
		resp, be := logic.NewExamAiGradeLogic(r.Context(), svcCtx).Trigger(sid, cu)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

// examAiGradeGetHandler GET /api/exam/submission/:sid/ai-grade
// 读一份考试提交的 AI 初评；processing 卡住时 logic 会去 ai-service 对账一次。
func examAiGradeGetHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		sid := class.ParseIntPath(r, "sid")
		view, be := logic.NewExamAiGradeLogic(r.Context(), svcCtx).Get(sid, cu)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		httpx.WriteJson(w, http.StatusOK, view)
	}
}
