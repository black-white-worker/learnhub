package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type GetActiveSigninLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetActiveSigninLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetActiveSigninLogic {
	return &GetActiveSigninLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *GetActiveSigninLogic) GetActiveSignin(classId int64, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	s, err := l.svcCtx.ClassStore.GetActiveSignin(classId)
	if err != nil {
		return nil, nil
	}
	return s, nil
}
