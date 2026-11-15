package user

import (
	"context"
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
	return FromContext(r.Context())
}

// FromContext 同 CurrentUser，但只依赖 context.Context。
// logic 层通常只拿到 context（r.Context() 传下来的），没有 *http.Request，
// 又需要知道「是谁在操作」（比如上传讲义后触发的知识库灌库要带发起人）。
func FromContext(ctx context.Context) *model.SessionUser {
	v := ctx.Value(AuthSessionKey)
	if v == nil {
		return nil
	}
	s, ok := v.(model.SessionUser)
	if !ok {
		return nil
	}
	return &s
}
