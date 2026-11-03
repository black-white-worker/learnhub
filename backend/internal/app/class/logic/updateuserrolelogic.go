package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type UpdateUserRoleLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUpdateUserRoleLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateUserRoleLogic {
	return &UpdateUserRoleLogic{ctx: ctx, svcCtx: svcCtx}
}

// UpdateUserRole 管理员修改用户角色。
// 问题3：审批（通过/驳回）后给被审批人写一条站内通知。
func (l *UpdateUserRoleLogic) UpdateUserRole(req *class.UpdateUserRoleReq, cu *class.CtxUser) error {
	if cu.Role != "admin" {
		return &class.BizError{Code: http.StatusForbidden, Msg: "仅管理员可修改用户角色"}
	}
	if req.Role != "admin" && req.Role != "teacher" && req.Role != "student" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "角色不合法"}
	}
	if err := l.svcCtx.UserStore.UpdateRole(req.Username, req.Role); err != nil {
		return &class.BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	// 给被审批人写通知
	if uid, err := l.svcCtx.UserStore.GetIdByUsername(req.Username); err == nil && uid > 0 {
		roleText := map[string]string{"admin": "管理员", "teacher": "老师", "student": "学生"}[req.Role]
		if _, nerr := l.svcCtx.ClassStore.InsertNotification(&model.NotificationRow{
			UserId:  uid,
			Type:    "approval",
			Title:   "老师申请审批结果",
			Content: "你的老师申请已" + map[bool]string{true: "", false: "被"}[req.Role == "teacher"] + "处理，当前角色：" + roleText,
			LinkUrl: "/profile",
		}); nerr == nil {
			// 实时推送：把"有新的审批结果"秒级推到铃铛（非阻塞，HTTP 立即返回）
			go l.svcCtx.NotifHub.Push(uid, []byte(`{"type":"unread","value":1}`))
		}
	}
	return nil
}
