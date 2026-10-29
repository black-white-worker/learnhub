package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type GradeSubmissionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGradeSubmissionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GradeSubmissionLogic {
	return &GradeSubmissionLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GradeSubmissionLogic) GradeSubmission(sid int64, req *class.GradeSubmissionReq, cu *class.CtxUser) error {
	sub, err := l.svcCtx.ClassStore.GetSubmission(sid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	a, err := l.svcCtx.ClassStore.GetAssignment(sub.AssignmentId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return be
	}
	if err := l.svcCtx.ClassStore.GradeSubmission(sid, req.Score, req.Comment); err != nil {
		return err
	}
	// 成绩册落库（对标 Canvas：分数在批改时写入，查询时只做聚合）
	l.recordGrade(a, sub.UserId, float64(req.Score), cu.Id)
	return nil
}

// recordGrade 把作业分数写进成绩册。失败只记日志，不影响批改本身。
func (l *GradeSubmissionLogic) recordGrade(a *model.AssignmentRow, userId int64, score float64, gradedBy int64) {
	maxScore := float64(a.TotalScore)
	if maxScore <= 0 {
		maxScore = 100
	}
	itemId, err := l.svcCtx.ClassStore.EnsureGradeItem(a.ClassId, "assignment", a.Id, a.Title, maxScore)
	if err != nil {
		logx.Errorf("写入成绩项失败 assignment=%d: %v", a.Id, err)
		return
	}
	if err := l.svcCtx.ClassStore.SetGradeScore(itemId, userId, score, gradedBy); err != nil {
		logx.Errorf("写入成绩失败 assignment=%d user=%d: %v", a.Id, userId, err)
	}
}
