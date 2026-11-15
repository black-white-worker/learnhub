package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// aiGradeTriggerHandler POST /api/assignment/submission/:sid/ai-grade
// 老师点「AI 初评」。载荷（题干/参考答案/学生作答/分值）全部由 logic 从库里取，
// 请求体没有任何业务字段，前端想塞也塞不进来。
func aiGradeTriggerHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		sid := class.ParseIntPath(r, "sid")
		resp, be := logic.NewAiGradeLogic(r.Context(), svcCtx).Trigger(sid, cu)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

// aiGradeGetHandler GET /api/assignment/submission/:sid/ai-grade
// 读一份提交的 AI 初评。卡在 processing 时 logic 会去 ai-service 对账一次，
// 前端只管按固定间隔轮询这个接口，不需要单独查任务状态。
func aiGradeGetHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		sid := class.ParseIntPath(r, "sid")
		view, be := logic.NewAiGradeLogic(r.Context(), svcCtx).Get(sid, cu)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		httpx.WriteJson(w, http.StatusOK, view)
	}
}

// aiGradeCallbackHandler POST /api/assignment/submission/:sid/ai-callback
// ai-service 批改 worker 的结果回写。鉴权是 X-Internal-Token（中间件层），
// 所以这里没有 CurrentUser：worker 手里没有用户 JWT。
//
// 这个接口只写 assignment_ai_grades（建议分），不写成绩册 —— 成绩册只能由老师落笔。
func aiGradeCallbackHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sid := class.ParseIntPath(r, "sid")
		var req class.AiGradeCallbackReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "回调数据格式不对"})
			return
		}
		// 路径里的 sid 是权威：body 里的 submitId 只用来做一致性校验，
		// 两者不一致说明 worker 拿错了任务，直接拒。
		if req.SubmitId != 0 && req.SubmitId != sid {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "回调的 submitId 和路径不一致"})
			return
		}
		if be := logic.NewAiGradeLogic(r.Context(), svcCtx).Callback(sid, &req); be != nil {
			class.WriteBizError(w, be)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "批改结果已接收"})
	}
}
