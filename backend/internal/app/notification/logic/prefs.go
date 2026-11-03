// Package logic 通知中心业务逻辑层。
package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/notification"
	"learningplatform/internal/svc"
)

// PrefsLogic 通知偏好相关的业务逻辑。
// 标准形态：持有 ctx 与 svcCtx，所有方法通过 svcCtx 访问存储。
type PrefsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewPrefsLogic 构造 PrefsLogic。
func NewPrefsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PrefsLogic {
	return &PrefsLogic{ctx: ctx, svcCtx: svcCtx}
}

// List 返回当前用户的全部渠道偏好。
// cu 由 handler 从登录态解析后传入，cu.Id 作为查询过滤条件。
func (l *PrefsLogic) List(cu *class.CtxUser) (*notification.PrefsResp, error) {
	rows, err := l.svcCtx.NotificationPrefStore.ListByUser(cu.Id)
	if err != nil {
		return nil, &class.BizError{Code: 500, Msg: err.Error()}
	}
	items := make([]notification.PrefItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, notification.PrefItem{Channel: r.Channel, Enabled: r.Enabled})
	}
	return &notification.PrefsResp{Prefs: items}, nil
}

// Update 新增或更新某渠道的偏好（Upsert 幂等）。
func (l *PrefsLogic) Update(cu *class.CtxUser, channel string, enabled int) error {
	if channel == "" {
		return &class.BizError{Code: 400, Msg: "channel 不能为空"}
	}
	return l.svcCtx.NotificationPrefStore.Upsert(cu.Id, channel, enabled)
}
