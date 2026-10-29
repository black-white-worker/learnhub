// Package middleware 全局 HTTP 中间件。
package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// requestIDKey 是 context 里存 request_id 的 key 类型，避免外部包冲突。
type requestIDKey struct{}

// RequestIDHeader 对外暴露的请求 ID header 名。
const RequestIDHeader = "X-Request-ID"

// RequestID 返回注入到 context 里的 request_id，没注入则返回空串。
// logic / handler 用它把 request_id 打进日志，串起一次请求的所有 log。
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey{}).(string); ok {
		return v
	}
	return ""
}

// RequestIDMiddleware 从 X-Request-ID 取或生成一个 request_id，
// 写回 response header，并注入 context 供下游使用。
// 签名匹配 go-zero 的 rest.Middleware：func(http.HandlerFunc) http.HandlerFunc。
func RequestIDMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(RequestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
		}
		w.Header().Set(RequestIDHeader, rid)
		ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
		next(w, r.WithContext(ctx))
	}
}
