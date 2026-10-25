package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListSigninsHistoryLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSigninsHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSigninsHistoryLogic {
	return &ListSigninsHistoryLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListSigninsHistory 班级签到历史列表（老师端）
func (l *ListSigninsHistoryLogic) ListSigninsHistory(classId int64, cu *class.CtxUser) (interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ClassStore.ListSigninsByClass(classId)
}
