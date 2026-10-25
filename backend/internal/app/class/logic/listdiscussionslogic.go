package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListDiscussionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListDiscussionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListDiscussionsLogic {
	return &ListDiscussionsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListDiscussionsLogic) ListDiscussions(classId int64, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ClassStore.ListDiscussions(classId, 20, 0)
}
