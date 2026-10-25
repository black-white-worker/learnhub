package logic

import (
	"context"
	"net/http"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type DoSigninLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDoSigninLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DoSigninLogic {
	return &DoSigninLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *DoSigninLogic) DoSignin(classId int64, req *class.DoSigninReq, cu *class.CtxUser) error {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return be
	}
	s, err := l.svcCtx.ClassStore.GetSigninByCode(req.Code)
	if err != nil || s.ClassId != classId {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "签到码错误"}
	}
	if time.Now().After(s.ExpiresAt) {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "签到已过期"}
	}
	inserted, err := l.svcCtx.ClassStore.AddSigninRecord(s.Id, cu.Id)
	if err != nil {
		return err
	}
	if !inserted {
		return &class.BizError{Code: http.StatusConflict, Msg: "你已经签到过了，请勿重复签到"}
	}
	return nil
}
