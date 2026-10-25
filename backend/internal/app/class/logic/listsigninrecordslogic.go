package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListSigninRecordsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListSigninRecordsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListSigninRecordsLogic {
	return &ListSigninRecordsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListSigninRecordsLogic) ListSigninRecords(classId int64, cu *class.CtxUser) (interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	s, err := l.svcCtx.ClassStore.GetActiveSignin(classId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "当前没有进行中的签到"}
	}
	return l.svcCtx.ClassStore.ListSigninRecords(s.Id)
}
