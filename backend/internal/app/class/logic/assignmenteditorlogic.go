package logic

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

// AssignmentEditorLogic 作业出题器：单题CRUD+排序+作业基本信息更新。
// 与 exam.ExamLogic 是平行结构，前端复用同一套 QuestionEditor 组件。
type AssignmentEditorLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignmentEditorLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignmentEditorLogic {
	return &AssignmentEditorLogic{ctx: ctx, svcCtx: svcCtx}
}

// validAssignmentTypes 作业允许的题型：填空/简答/文件上传为主，也支持单选/多选。
var validAssignmentTypes = map[string]bool{
	"single": true, "multiple": true, "fill": true, "essay": true, "fileupload": true,
}

// AddQuestion 老师给作业加一道空题卡。
func (l *AssignmentEditorLogic) AddQuestion(aid int64, req *class.AddQuestionReq, cu *class.CtxUser) (*class.AddQuestionResp, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, be
	}
	qt := req.Type
	if qt == "" {
		qt = "essay"
	}
	if !validAssignmentTypes[qt] {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "作业题型不支持"}
	}
	score := req.Score
	if score <= 0 {
		score = 5
	}
	required := 1
	if req.Required != nil {
		required = *req.Required
	}
	maxOrder, _ := l.svcCtx.ClassStore.MaxAssignmentQuestionOrder(aid)
	id, err := l.svcCtx.ClassStore.InsertQuestion(&model.QuestionRow{
		AssignmentId: aid, Type: qt, Stem: req.Stem, Options: req.Options, Answer: req.Answer,
		Score: score, OrderNum: maxOrder + 1, Required: required,
	})
	if err != nil {
		return nil, err
	}
	return &class.AddQuestionResp{Id: id}, nil
}

// UpdateQuestion 编辑作业单题。
func (l *AssignmentEditorLogic) UpdateQuestion(qid int64, req *class.UpdateAssignmentQuestionReq, cu *class.CtxUser) error {
	q, err := l.svcCtx.ClassStore.GetAssignmentQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	a, err := l.svcCtx.ClassStore.GetAssignment(q.AssignmentId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
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
	return l.svcCtx.ClassStore.UpdateAssignmentQuestion(&model.QuestionRow{
		Id: qid, AssignmentId: q.AssignmentId, Type: qt, Stem: req.Stem, Options: req.Options,
		Answer: req.Answer, Score: score, Required: required,
	})
}

// DeleteQuestion 删作业题。
func (l *AssignmentEditorLogic) DeleteQuestion(qid int64, cu *class.CtxUser) error {
	q, err := l.svcCtx.ClassStore.GetAssignmentQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	a, err := l.svcCtx.ClassStore.GetAssignment(q.AssignmentId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return be
	}
	return l.svcCtx.ClassStore.DeleteAssignmentQuestion(qid)
}

// SortQuestions 拖拽排序。
func (l *AssignmentEditorLogic) SortQuestions(aid int64, req *class.SortAssignmentQuestionsReq, cu *class.CtxUser) error {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return be
	}
	if len(req.Order) == 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "排序数组不能为空"}
	}
	return l.svcCtx.ClassStore.SortAssignmentQuestions(aid, req.Order)
}

// UpdateAssignment 更新作业基本信息。
func (l *AssignmentEditorLogic) UpdateAssignment(aid int64, req *class.UpdateAssignmentReq, cu *class.CtxUser) error {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return be
	}
	if req.Title == "" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "作业标题不能为空"}
	}
	var dl sql.NullTime
	if req.Deadline != "" {
		if t, err := time.Parse("2006-01-02T15:04", req.Deadline); err == nil {
			dl = sql.NullTime{Time: t, Valid: true}
		}
	} else {
		dl = a.Deadline
	}
	var ldl sql.NullTime
	if req.LateDeadline != "" {
		if t, err := time.Parse("2006-01-02T15:04", req.LateDeadline); err == nil {
			ldl = sql.NullTime{Time: t, Valid: true}
		}
	}
	score := req.TotalScore
	if score <= 0 {
		score = a.TotalScore
	}
	return l.svcCtx.ClassStore.UpdateAssignment(&model.AssignmentRow{
		Id: aid, Title: req.Title, Description: req.Description,
		Deadline: dl, LateDeadline: ldl, TotalScore: score, AllowLate: req.AllowLate,
	})
}
