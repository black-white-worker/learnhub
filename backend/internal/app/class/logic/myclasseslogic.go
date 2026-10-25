package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type MyClassesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMyClassesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MyClassesLogic {
	return &MyClassesLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *MyClassesLogic) MyClasses(cu *class.CtxUser) (interface{}, error) {
	return l.svcCtx.ClassStore.ListClassesByUser(cu.Id)
}
