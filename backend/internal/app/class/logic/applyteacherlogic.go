package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ApplyTeacherLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewApplyTeacherLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ApplyTeacherLogic {
	return &ApplyTeacherLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ApplyTeacherLogic) ApplyTeacher(cu *class.CtxUser) error {
	return l.svcCtx.UserStore.ApplyTeacher(cu.Username)
}
