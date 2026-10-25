package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type CreateInviteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateInviteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateInviteLogic {
	return &CreateInviteLogic{ctx: ctx, svcCtx: svcCtx}
}

// CreateInvite 生成班级邀请码。
// 问题9：邀请码只能生成一次——如果该班已有邀请码，直接返回旧码，不重复生成。
func (l *CreateInviteLogic) CreateInvite(classId int64, cu *class.CtxUser) (*class.CreateInviteResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	// 已有邀请码则直接返回旧码，不重复生成
	if existing, err := l.svcCtx.ClassStore.GetActiveInvitation(classId); err == nil && existing != nil {
		return &class.CreateInviteResp{Code: existing.Code}, nil
	}
	code := class.Rand6Code()
	if err := l.svcCtx.ClassStore.CreateInvitation(classId, code); err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	return &class.CreateInviteResp{Code: code}, nil
}
