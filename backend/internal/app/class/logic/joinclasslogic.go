package logic

import (
	"context"
	"net/http"
	"strings"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type JoinClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewJoinClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *JoinClassLogic {
	return &JoinClassLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *JoinClassLogic) JoinClass(req *class.JoinClassReq, cu *class.CtxUser) (*class.JoinClassResp, error) {
	inv, err := l.svcCtx.ClassStore.GetInvitationByCode(strings.TrimSpace(req.Code))
	if err != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "邀请码无效"}
	}
	if inv.ExpiresAt.Valid && inv.ExpiresAt.Time.Before(time.Now()) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "邀请码已过期"}
	}
	if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "邀请码已达使用上限"}
	}
	if err := l.svcCtx.ClassStore.AddMember(inv.ClassId, cu.Id, "student"); err != nil {
		return nil, err
	}
	_ = l.svcCtx.ClassStore.IncInvitationUse(inv.Id)
	return &class.JoinClassResp{ClassId: inv.ClassId, Message: "加入成功"}, nil
}
