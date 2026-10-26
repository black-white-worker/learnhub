// Package handler 考试中心 HTTP 入口
package handler

import (
	"fmt"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/exam"
	"learningplatform/internal/app/exam/logic"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	routes := []rest.Route{
		// 创建考试
		{Method: http.MethodPost, Path: "/api/class/:id/exam", Handler: createExamHandler(serverCtx)},
		// 班级考试列表（分页）
		{Method: http.MethodGet, Path: "/api/class/:id/exams", Handler: listExamsHandler(serverCtx)},
		// 考试详情
		{Method: http.MethodGet, Path: "/api/exam/:id", Handler: examDetailHandler(serverCtx)},
		// 更新考试基本信息
		{Method: http.MethodPut, Path: "/api/exam/:id", Handler: updateExamHandler(serverCtx)},
		// 给考试加题（单题，空题卡）
		{Method: http.MethodPost, Path: "/api/exam/:id/question", Handler: addQuestionHandler(serverCtx)},
		// 兼容旧路径
		{Method: http.MethodPost, Path: "/api/exam/:id/questions", Handler: addQuestionHandler(serverCtx)},
		// 批量组卷（老师一次加多道题，保留但前端不用）
		{Method: http.MethodPost, Path: "/api/exam/:id/questions/batch", Handler: batchAddQuestionsHandler(serverCtx)},
		// 编辑单题
		{Method: http.MethodPut, Path: "/api/exam/question/:qid", Handler: updateQuestionHandler(serverCtx)},
		// 删除单题
		{Method: http.MethodDelete, Path: "/api/exam/question/:qid", Handler: deleteQuestionHandler(serverCtx)},
		// 拖拽排序
		{Method: http.MethodPut, Path: "/api/exam/question/:qid/sort", Handler: sortQuestionsHandler(serverCtx)},
		// 学生开考
		{Method: http.MethodPost, Path: "/api/exam/:id/start", Handler: startExamHandler(serverCtx)},
		// 交卷
		{Method: http.MethodPost, Path: "/api/exam/:id/submit", Handler: submitHandler(serverCtx)},
		// 老师手批
		{Method: http.MethodPost, Path: "/api/exam/submission/:sid/grade", Handler: gradeHandler(serverCtx)},
	}
	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, routes...),
	)
}

func createExamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		var req exam.CreateExamReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).CreateExam(classId, &req, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

func listExamsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		page := 1
		size := 20
		fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page)
		fmt.Sscanf(r.URL.Query().Get("pageSize"), "%d", &size)
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).ListExams(classId, page, size, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

func examDetailHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).Detail(examId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

func addQuestionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		var req exam.AddQuestionReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).AddQuestion(examId, &req, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

func submitHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		var req exam.SubmitReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).Submit(examId, &req, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

func gradeHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		sid := class.ParseIntPath(r, "sid")
		var req exam.GradeReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewExamLogic(r.Context(), svcCtx).Grade(sid, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "批改成功"})
	}
}

// startExamHandler 学生点"开始考试"，写 started_at，返回截止时间前端倒计时。
func startExamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).StartExam(examId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

// batchAddQuestionsHandler 老师批量组卷。
func batchAddQuestionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		var req exam.BatchAddQuestionsReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).BatchAddQuestions(examId, &req, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

// updateExamHandler 老师更新考试基本信息。
func updateExamHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		examId := class.ParseIntPath(r, "id")
		var req exam.UpdateExamReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewExamLogic(r.Context(), svcCtx).UpdateExam(examId, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "保存成功"})
	}
}

// updateQuestionHandler 编辑单题。
func updateQuestionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		qid := class.ParseIntPath(r, "qid")
		var req exam.UpdateQuestionReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewExamLogic(r.Context(), svcCtx).UpdateQuestion(qid, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "保存成功"})
	}
}

// deleteQuestionHandler 删题。
func deleteQuestionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		qid := class.ParseIntPath(r, "qid")
		if err := logic.NewExamLogic(r.Context(), svcCtx).DeleteQuestion(qid, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "已删除"})
	}
}

// sortQuestionsHandler 拖拽排序。注意路径是 /question/:qid/sort，这里 qid 占位不用，从 body 取 order 数组。
func sortQuestionsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		// 从 qid 反查 examId（题目所属考试）
		qid := class.ParseIntPath(r, "qid")
		var req exam.SortQuestionsReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		// 先通过 qid 拿到 examId
		q, err := svcCtx.ExamStore.GetQuestion(qid)
		if err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"})
			return
		}
		if err := logic.NewExamLogic(r.Context(), svcCtx).SortQuestions(q.ExamId, &req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "排序已保存"})
	}
}
