package logic

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type SubmitAssignmentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSubmitAssignmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SubmitAssignmentLogic {
	return &SubmitAssignmentLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *SubmitAssignmentLogic) SubmitAssignment(aid int64, req *class.SubmitAssignmentReq, cu *class.CtxUser) (*class.SubmitAssignmentResp, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, a.ClassId, cu)
	if be != nil {
		return nil, be
	}
	if role == "teacher" || role == "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "老师/创建者无需提交作业"}
	}
	if a.Deadline.Valid && a.Deadline.Time.Before(time.Now()) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "已超过截止时间，不能提交"}
	}
	if err := class.ValidateSubmissionInput(req.Content, req.FileUrl, req.Answers); err != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: err.Error()}
	}
	if err := l.svcCtx.ClassStore.UpsertSubmission(&model.SubmissionRow{
		AssignmentId: aid, UserId: cu.Id, Content: req.Content, FileUrl: req.FileUrl,
	}); err != nil {
		return nil, err
	}
	resp := &class.SubmitAssignmentResp{Message: "提交成功"}
	if len(req.Answers) > 0 {
		questions, _ := l.svcCtx.ClassStore.ListQuestions(aid)
		autoScore := class.GradeAutoAnswers(questions, req.Answers)
		answersJSON, _ := json.Marshal(req.Answers)
		sub, _ := l.svcCtx.ClassStore.GetMySubmission(aid, cu.Id)
		if sub != nil {
			_ = l.svcCtx.ClassStore.UpdateSubmissionAutoScore(sub.Id, string(answersJSON), autoScore)
		}
		resp.AutoScore = autoScore

		// 成绩册落库：客观题自动判分直接记一条（graded_by=0 表示系统判分）。
		// 老师后续手动批改会覆盖这条记录。
		maxScore := float64(a.TotalScore)
		if maxScore <= 0 {
			maxScore = float64(class.SumQuestionScore(questions))
		}
		if maxScore > 0 {
			if itemId, e := l.svcCtx.ClassStore.EnsureGradeItem(a.ClassId, "assignment", a.Id, a.Title, maxScore); e == nil {
				_ = l.svcCtx.ClassStore.SetGradeScore(itemId, cu.Id, float64(autoScore), 0)
			}
		}
	}
	return resp, nil
}
