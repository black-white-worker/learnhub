package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type ApplyTeacherLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewApplyTeacherLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ApplyTeacherLogic {
	return &ApplyTeacherLogic{ctx: ctx, svcCtx: svcCtx}
}

// ApplyTeacher 学生申请成为老师。
// 问题2：申请成功后给所有管理员写一条通知，让管理员能收到审批提醒。
func (l *ApplyTeacherLogic) ApplyTeacher(cu *class.CtxUser) error {
	if err := l.svcCtx.UserStore.ApplyTeacher(cu.Username); err != nil {
		return &class.BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	// 给所有 admin 用户发通知
	users, err := l.svcCtx.UserStore.ListAll()
	if err == nil {
		for _, u := range users {
			if u.Role == "admin" {
				if uid, e := l.svcCtx.UserStore.GetIdByUsername(u.Username); e == nil && uid > 0 {
					if _, nerr := l.svcCtx.ClassStore.InsertNotification(&model.NotificationRow{
						UserId:  uid,
						Type:    "approval",
						Title:   "新的老师申请待审批",
						Content: "学生 " + cu.Username + " 申请成为老师，请前往用户管理审批",
						LinkUrl: "/profile",
					}); nerr == nil {
						// 实时推送：通知管理员有待审批申请（非阻塞，HTTP 立即返回）
						go l.svcCtx.NotifHub.Push(uid, []byte(`{"type":"unread","value":1}`))
					}
				}
			}
		}
	}
	return nil
}
