package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type AllClassesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAllClassesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AllClassesLogic {
	return &AllClassesLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *AllClassesLogic) AllClasses(cu *class.CtxUser) (interface{}, error) {
	if cu.Role != "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可查看所有班级"}
	}
	return l.svcCtx.ClassStore.ListAllClasses()
}
