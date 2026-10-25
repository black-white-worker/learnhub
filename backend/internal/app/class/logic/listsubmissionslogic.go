package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListSubmissionsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSubmissionsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSubmissionsLogic {
	return &ListSubmissionsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListSubmissionsLogic) ListSubmissions(aid int64, cu *class.CtxUser) (interface{}, error) {
	a, err := l.svcCtx.ClassStore.GetAssignment(aid)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, be
	}
	return l.svcCtx.ClassStore.ListSubmissions(aid)
}
