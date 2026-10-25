package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
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
	return l.svcCtx.ClassStore.GradeSubmission(sid, req.Score, req.Comment)
}
