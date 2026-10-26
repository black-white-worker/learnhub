package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// updateAssignmentHandler 更新作业基本信息
func updateAssignmentHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		aid := class.ParseIntPath(r, "aid")
		var req class.UpdateAssignmentReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewAssignmentEditorLogic(r.Context(), svcCtx).UpdateAssignment(aid, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "保存成功"})
	}
}

// updateAssignmentQuestionHandler 编辑作业单题
func updateAssignmentQuestionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		qid := class.ParseIntPath(r, "qid")
		var req class.UpdateAssignmentQuestionReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewAssignmentEditorLogic(r.Context(), svcCtx).UpdateQuestion(qid, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "保存成功"})
	}
}

// deleteAssignmentQuestionHandler 删作业题
func deleteAssignmentQuestionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		qid := class.ParseIntPath(r, "qid")
		if err := logic.NewAssignmentEditorLogic(r.Context(), svcCtx).DeleteQuestion(qid, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "已删除"})
	}
}

// sortAssignmentQuestionsHandler 作业题拖拽排序
func sortAssignmentQuestionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		qid := class.ParseIntPath(r, "qid")
		var req class.SortAssignmentQuestionsReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		q, err := svcCtx.ClassStore.GetAssignmentQuestion(qid)
		if err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"})
			return
		}
		if err := logic.NewAssignmentEditorLogic(r.Context(), svcCtx).SortQuestions(q.AssignmentId, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "排序已保存"})
	}
}
