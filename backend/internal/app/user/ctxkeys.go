package user

import (
	"net/http"

	"learningplatform/internal/model"
)

// ctxKey 自定义上下文键类型，避免与其他包使用的键冲突。
type ctxKey int

const (
	// authTokenKey 鉴权中间件放入 context 的登录 token
	AuthTokenKey ctxKey = iota
	// AuthSessionKey 鉴权中间件放入 context 的会话信息
	AuthSessionKey
	// ClientIPKey 注册/登录等场景放入 context 的客户端 IP（用于限流）
	ClientIPKey
)

// CurrentUser 从请求 context 取出当前登录用户（鉴权中间件已写入）。
// 未登录时返回 nil。其他模块（class/course）通过这个函数获取当前用户。
func CurrentUser(r *http.Request) *model.SessionUser {
	v := r.Context().Value(AuthSessionKey)
	if v == nil {
		return nil
	}
	s, ok := v.(model.SessionUser)
	if !ok {
		return nil
	}
	return &s
}
