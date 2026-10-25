package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListAllUsersLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAllUsersLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAllUsersLogic {
	return &ListAllUsersLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAllUsersLogic) ListAllUsers(cu *class.CtxUser) (interface{}, error) {
	if cu.Role != "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可查看用户列表"}
	}
	return l.svcCtx.UserStore.ListAll()
}
