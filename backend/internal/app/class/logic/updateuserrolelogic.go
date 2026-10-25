package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type UpdateUserRoleLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserRoleLogic {
	return &UpdateUserRoleLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *UpdateUserRoleLogic) UpdateUserRole(req *class.UpdateUserRoleReq, cu *class.CtxUser) error {
	if cu.Role != "admin" {
		return &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可修改用户角色"}
	}
	if req.Role != "admin" && req.Role != "teacher" && req.Role != "student" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "角色不合法"}
	}
	return l.svcCtx.UserStore.UpdateRole(req.Username, req.Role)
}
