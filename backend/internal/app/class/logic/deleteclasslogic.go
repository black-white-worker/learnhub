package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type DeleteClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteClassLogic {
	return &DeleteClassLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteClassLogic) DeleteClass(classId int64, cu *class.CtxUser) error {
	if cu.Role != "admin" {
		return &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可删除班级"}
	}
	return l.svcCtx.ClassStore.DeleteClass(classId)
}
