package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListAssignmentsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAssignmentsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAssignmentsLogic {
	return &ListAssignmentsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAssignmentsLogic) ListAssignments(classId int64, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ClassStore.ListAssignments(classId)
}
