package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListNotificationsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListNotificationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListNotificationsLogic {
	return &ListNotificationsLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListNotifications 返回当前用户的通知列表 + 未读数
func (l *ListNotificationsLogic) ListNotifications(cu *class.CtxUser) (interface{}, error) {
	list, err := l.svcCtx.ClassStore.ListNotifications(cu.Id, 50)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	unread, _ := l.svcCtx.ClassStore.CountUnreadNotifications(cu.Id)
	return map[string]interface{}{
		"list":   list,
		"unread": unread,
	}, nil
}

type MarkNotificationReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkNotificationReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkNotificationReadLogic {
	return &MarkNotificationReadLogic{ctx: ctx, svcCtx: svcCtx}
}

// MarkOne 标记单条已读
func (l *MarkNotificationReadLogic) MarkOne(id int64, cu *class.CtxUser) error {
	return l.svcCtx.ClassStore.MarkNotificationRead(id, cu.Id)
}

// MarkAll 全部已读
func (l *MarkNotificationReadLogic) MarkAll(cu *class.CtxUser) error {
	return l.svcCtx.ClassStore.MarkAllNotificationsRead(cu.Id)
}
