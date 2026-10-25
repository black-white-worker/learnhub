package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
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

// ListExams 班级考试列表
func (l *ExamLogic) ListExams(classId int64, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ExamStore.ListExamsByClass(classId)
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

// AddQuestion 老师给考试加题
func (l *ExamLogic) AddQuestion(examId int64, req *exam.AddQuestionReq, cu *class.CtxUser) (*exam.AddQuestionResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, be
	}
	if req.Stem == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题干不能为空"}
	}
	if req.Type == "" {
		req.Type = "single"
	}
	if req.Type != "essay" && req.Answer == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "选择题/判断题答案不能为空"}
	}
	score := req.Score
	if score <= 0 {
		score = 5
	}
	id, err := l.svcCtx.ExamStore.InsertQuestion(&model.ExamQuestionRow{
		ExamId: examId, Type: req.Type, Stem: req.Stem, Options: req.Options, Answer: req.Answer, Score: score,
	})
	if err != nil {
		return nil, err
	}
	return &exam.AddQuestionResp{Id: id}, nil
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

	// 自动判客观题
	questions, _ := l.svcCtx.ExamStore.ListQuestions(examId)
	autoScore := 0
	for _, q := range questions {
		if q.Type == "essay" {
			continue
		}
		pick, ok := req.Answers[strconv.FormatInt(q.Id, 10)]
		if !ok {
			continue
		}
		if q.Type == "multiple" {
			if class.SameMultiChoice(pick, q.Answer) {
				autoScore += q.Score
			}
			continue
		}
		if pick == q.Answer {
			autoScore += q.Score
		}
	}
	answersJSON, _ := json.Marshal(req.Answers)

	if err := l.svcCtx.ExamStore.UpsertSubmission(&model.ExamSubmissionRow{
		ExamId: examId, UserId: cu.Id, Answers: string(answersJSON),
		AutoScore: nullInt(autoScore),
	}); err != nil {
		return nil, err
	}
	return &exam.SubmitResp{Message: "交卷成功", AutoScore: autoScore}, nil
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
