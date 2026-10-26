package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/exam"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type ExamLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExamLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExamLogic {
	return &ExamLogic{ctx: ctx, svcCtx: svcCtx}
}

// CreateExam 老师创建考试
func (l *ExamLogic) CreateExam(classId int64, req *exam.CreateExamReq, cu *class.CtxUser) (*exam.CreateExamResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	if req.Title == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试标题不能为空"}
	}
	start, err1 := time.ParseInLocation("2006-01-02T15:04", req.StartAt, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02T15:04", req.EndAt, time.Local)
	if err1 != nil || err2 != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "开始/结束时间格式应为 YYYY-MM-DDTHH:mm"}
	}
	if !end.After(start) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "结束时间必须晚于开始时间"}
	}
	dur := req.DurationMin
	if dur <= 0 {
		dur = 60
	}
	total := req.TotalScore
	if total <= 0 {
		total = 100
	}
	id, err := l.svcCtx.ExamStore.InsertExam(&model.ExamRow{
		ClassId: classId, Title: req.Title, Description: req.Description,
		StartAt: start, EndAt: end, DurationMin: dur, TotalScore: total, CreatedBy: cu.Id,
	})
	if err != nil {
		return nil, err
	}
	return &exam.CreateExamResp{Id: id}, nil
}

// ListExams 班级考试列表（分页）
func (l *ExamLogic) ListExams(classId int64, page, size int, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	list, total, err := l.svcCtx.ExamStore.ListExamsByClassPaged(classId, page, size)
	if err != nil {
		return nil, err
	}
	return &exam.PagedResp{List: list, Total: total, Page: page, PageSize: size}, nil
}

// Detail 考试详情：学生看题目+自己提交，老师看提交列表
func (l *ExamLogic) Detail(examId int64, cu *class.CtxUser) (interface{}, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	questions, _ := l.svcCtx.ExamStore.ListQuestions(examId)

	// 学生：返回自己的提交；老师：返回所有人提交
	if role == "teacher" || role == "admin" {
		subs, _ := l.svcCtx.ExamStore.ListSubmissions(examId)
		return map[string]interface{}{
			"exam":         e,
			"questions":    questions,
			"submissions":  subs,
			"myRole":       role,
		}, nil
	}

	sub, _ := l.svcCtx.ExamStore.GetMySubmission(examId, cu.Id)
	// 学生端不返回答案
	for _, q := range questions {
		q.Answer = ""
	}
	return map[string]interface{}{
		"exam":         e,
		"questions":    questions,
		"mySubmission": sub,
		"myRole":       role,
	}, nil
}

// AddQuestion 老师给考试加题（空题卡，点题型栏追加）。
// 对标腾讯问卷：题干可以先空着，后端只做最基本校验。
func (l *ExamLogic) AddQuestion(examId int64, req *exam.AddQuestionReq, cu *class.CtxUser) (*exam.AddQuestionResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, be
	}
	qt := req.Type
	if qt == "" {
		qt = "single"
	}
	// 允许的题型：single单选 multiple多选 judge判断 fill填空 essay简答 fileupload文件上传
	allowed := map[string]bool{"single": true, "multiple": true, "judge": true, "fill": true, "essay": true, "fileupload": true}
	if !allowed[qt] {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题型不支持"}
	}
	score := req.Score
	if score <= 0 {
		score = 5
	}
	// 新题追加到末尾
	maxOrder, _ := l.svcCtx.ExamStore.MaxOrderNum(examId)
	id, err := l.svcCtx.ExamStore.InsertQuestion(&model.ExamQuestionRow{
		ExamId: examId, Type: qt, Stem: req.Stem, Options: req.Options, Answer: req.Answer,
		Score: score, OrderNum: maxOrder + 1, Required: 1,
	})
	if err != nil {
		return nil, err
	}
	return &exam.AddQuestionResp{Id: id}, nil
}

// UpdateQuestion 编辑单题（题干/选项/答案/分值/必答）。
func (l *ExamLogic) UpdateQuestion(qid int64, req *exam.UpdateQuestionReq, cu *class.CtxUser) error {
	q, err := l.svcCtx.ExamStore.GetQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(q.ExamId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	required := q.Required
	if req.Required != nil {
		required = *req.Required
	}
	score := req.Score
	if score <= 0 {
		score = q.Score
	}
	qt := req.Type
	if qt == "" {
		qt = q.Type
	}
	return l.svcCtx.ExamStore.UpdateQuestion(&model.ExamQuestionRow{
		Id: qid, ExamId: q.ExamId, Type: qt, Stem: req.Stem, Options: req.Options,
		Answer: req.Answer, Score: score, Required: required,
	})
}

// DeleteQuestion 删题。
func (l *ExamLogic) DeleteQuestion(qid int64, cu *class.CtxUser) error {
	q, err := l.svcCtx.ExamStore.GetQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(q.ExamId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	return l.svcCtx.ExamStore.DeleteQuestion(qid)
}

// SortQuestions 拖拽排序。
func (l *ExamLogic) SortQuestions(examId int64, req *exam.SortQuestionsReq, cu *class.CtxUser) error {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	if len(req.Order) == 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "排序数组不能为空"}
	}
	return l.svcCtx.ExamStore.SortQuestions(examId, req.Order)
}

// UpdateExam 更新考试基本信息。
func (l *ExamLogic) UpdateExam(examId int64, req *exam.UpdateExamReq, cu *class.CtxUser) error {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	if req.Title == "" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "考试标题不能为空"}
	}
	start, err1 := time.ParseInLocation("2006-01-02T15:04", req.StartAt, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02T15:04", req.EndAt, time.Local)
	if err1 != nil || err2 != nil {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "开始/结束时间格式应为 YYYY-MM-DDTHH:mm"}
	}
	if !end.After(start) {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "结束时间必须晚于开始时间"}
	}
	dur := req.DurationMin
	if dur <= 0 {
		dur = e.DurationMin
	}
	total := req.TotalScore
	if total <= 0 {
		total = e.TotalScore
	}
	return l.svcCtx.ExamStore.UpdateExam(&model.ExamRow{
		Id: examId, Title: req.Title, Description: req.Description,
		StartAt: start, EndAt: end, DurationMin: dur, TotalScore: total,
		AllowLate: req.AllowLate, TabLimit: req.TabLimit,
	})
}

// Submit 学生交卷
func (l *ExamLogic) Submit(examId int64, req *exam.SubmitReq, cu *class.CtxUser) (*exam.SubmitResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	if role == "teacher" || role == "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "老师无需参加考试"}
	}
	now := time.Now()
	if now.Before(e.StartAt) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试还未开始"}
	}
	if now.After(e.EndAt) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试已结束"}
	}

	// 自动判客观题（抽成纯函数 scoreExam，可单测）
	questions, _ := l.svcCtx.ExamStore.ListQuestions(examId)
	scoreRes := ScoreExam(questions, req.Answers)
	answersJSON, _ := json.Marshal(req.Answers)

	if err := l.svcCtx.ExamStore.UpsertSubmission(&model.ExamSubmissionRow{
		ExamId: examId, UserId: cu.Id, Answers: string(answersJSON),
		AutoScore: nullInt(scoreRes.AutoScore),
	}); err != nil {
		return nil, err
	}
	return &exam.SubmitResp{Message: "交卷成功", AutoScore: scoreRes.AutoScore}, nil
}

// Grade 老师手批
func (l *ExamLogic) Grade(submissionId int64, req *exam.GradeReq, cu *class.CtxUser) error {
	// 简单校验：submission 属于当前老师所教班级的考试
	// 这里直接更新，鉴权由 handler 层的老师权限兜底（简化：老师能批任何考试提交）
	if req.TeacherScore < 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "分数不能为负"}
	}
	return l.svcCtx.ExamStore.GradeSubmission(submissionId, req.TeacherScore, req.Comment)
}

func nullInt(v int) (r sql.NullInt64) {
	r.Int64 = int64(v)
	r.Valid = true
	return
}

// StartExam 学生点"开始考试"：在 exam_submissions 写一行 started_at=now。
// 后续交卷 UpsertSubmission 会基于唯一键 (exam_id,user_id) 更新 answers/auto_score。
// 重复调用不会重置 started_at（model 层用 INSERT IGNORE）。
func (l *ExamLogic) StartExam(examId int64, cu *class.CtxUser) (*exam.StartExamResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	if role == "teacher" || role == "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "老师无需参加考试"}
	}
	now := time.Now()
	if now.Before(e.StartAt) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试还未开始"}
	}
	if now.After(e.EndAt) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试已结束"}
	}
	if err := l.svcCtx.ExamStore.StartSubmission(examId, cu.Id, now); err != nil {
		return nil, err
	}
	// 截止时间 = min(end_at, started_at + duration)
	deadline := e.EndAt
	latestStart := e.StartAt.Add(time.Duration(e.DurationMin) * time.Minute)
	if latestStart.Before(deadline) {
		deadline = latestStart
	}
	return &exam.StartExamResp{
		ExamId:      examId,
		StartedAt:   now.Format(time.RFC3339),
		DeadlineAt:  deadline.Format(time.RFC3339),
		DurationMin: e.DurationMin,
	}, nil
}

// BatchAddQuestions 老师一次给考试加多道题（手动组卷）。
// 与 AddQuestion 单题接口互补：整份试卷录入时用这个接口，省 N 次 RTT。
func (l *ExamLogic) BatchAddQuestions(examId int64, req *exam.BatchAddQuestionsReq, cu *class.CtxUser) (*exam.BatchAddQuestionsResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, be
	}
	if len(req.Questions) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题目列表不能为空"}
	}
	rows := make([]*model.ExamQuestionRow, 0, len(req.Questions))
	for _, q := range req.Questions {
		if q.Stem == "" {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题干不能为空"}
		}
		qt := q.Type
		if qt == "" {
			qt = "single"
		}
		if qt != "essay" && q.Answer == "" {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "选择题/判断题答案不能为空"}
		}
		score := q.Score
		if score <= 0 {
			score = 5
		}
		rows = append(rows, &model.ExamQuestionRow{
			ExamId: examId, Type: qt, Stem: q.Stem, Options: q.Options, Answer: q.Answer, Score: score,
		})
	}
	ids, err := l.svcCtx.ExamStore.BatchInsertQuestions(rows)
	if err != nil {
		return nil, err
	}
	return &exam.BatchAddQuestionsResp{Count: len(ids), Ids: ids}, nil
}
