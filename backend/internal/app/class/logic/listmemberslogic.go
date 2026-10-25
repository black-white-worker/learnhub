package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListMembersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMembersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMembersLogic {
	return &ListMembersLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListMembersLogic) ListMembers(classId int64, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ClassStore.ListMembers(classId)
}
