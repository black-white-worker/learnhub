// Package handler 考试中心 HTTP 入口
package handler

import (
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
		// 班级考试列表
		{Method: http.MethodGet, Path: "/api/class/:id/exams", Handler: listExamsHandler(serverCtx)},
		// 考试详情
		{Method: http.MethodGet, Path: "/api/exam/:id", Handler: examDetailHandler(serverCtx)},
		// 给考试加题（单题）
		{Method: http.MethodPost, Path: "/api/exam/:id/questions", Handler: addQuestionHandler(serverCtx)},
		// 批量组卷（老师一次加多道题）
		{Method: http.MethodPost, Path: "/api/exam/:id/questions/batch", Handler: batchAddQuestionsHandler(serverCtx)},
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
		resp, err := logic.NewExamLogic(r.Context(), svcCtx).ListExams(classId, cu)
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
