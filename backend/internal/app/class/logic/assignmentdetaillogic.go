package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type AssignmentDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAssignmentDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AssignmentDetailLogic {
	return &AssignmentDetailLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *AssignmentDetailLogic) AssignmentDetail(aid int64, cu *class.CtxUser) (*class.AssignmentDetailResp, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if _, be := class.RequireClassMember(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, be
	}
	questions, _ := l.svcCtx.ClassStore.ListQuestions(aid)
	sub, _ := l.svcCtx.ClassStore.GetMySubmission(aid, cu.Id)
	// 学生端不返回答案，老师/管理员端才返回
	if cu.Role != "admin" && cu.Role != "teacher" {
		for _, q := range questions {
			q.Answer = ""
		}
	}
	return &class.AssignmentDetailResp{Assignment: a, Questions: questions, MySubmission: sub}, nil
}
