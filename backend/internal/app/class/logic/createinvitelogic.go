package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type CreateInviteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateInviteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateInviteLogic {
	return &CreateInviteLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateInviteLogic) CreateInvite(classId int64, cu *class.CtxUser) (*class.CreateInviteResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	code := class.Rand6Code()
	if err := l.svcCtx.ClassStore.CreateInvitation(classId, code); err != nil {
		return nil, err
	}
	return &class.CreateInviteResp{Code: code}, nil
}
