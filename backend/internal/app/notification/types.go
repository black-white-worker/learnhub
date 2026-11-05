// Package notification 通知中心业务模块。
//
// 本模块包含两部分：
//  1. 通知偏好（REST）：列出 / 更新用户接收通知的渠道开关；
//  2. 实时推送（WebSocket）：/ws/notifications 长连接，由 platform/ws.Hub 推送新消息。
//
// 通知消息本体（列表 / 未读 / 已读）由 class 模块实现，本模块只补"偏好"与"实时"。
package notification

// PrefItem 单条渠道偏好，如 {channel:"inapp", enabled:1}。
type PrefItem struct {
	Channel string `json:"channel"`
	Enabled int    `json:"enabled"`
}

// PrefsResp 偏好列表接口的返回体。
type PrefsResp struct {
	Prefs []PrefItem `json:"prefs"`
}

// UpdatePrefReq 更新偏好的请求体（前端 POST 来的 JSON）。
type UpdatePrefReq struct {
	Channel string `json:"channel"` // 必填：inapp / email
	Enabled int    `json:"enabled"` // 1=开启，0=关闭
}
