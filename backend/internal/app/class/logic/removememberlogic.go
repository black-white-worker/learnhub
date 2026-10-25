package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type RemoveMemberLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRemoveMemberLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RemoveMemberLogic {
	return &RemoveMemberLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *RemoveMemberLogic) RemoveMember(classId, userId int64, cu *class.CtxUser) error {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return be
	}
	return l.svcCtx.ClassStore.RemoveMember(classId, userId)
}
