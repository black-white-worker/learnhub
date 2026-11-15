// 服务间回调鉴权：ai-service → Go 的回调只带 X-Internal-Token，没有用户 JWT。
//
// 为什么单独一个中间件而不是复用登录态：
// 批改 worker 是后台进程，手里没有学生的 token；让它持有一个共享密钥，
// 比给它签发"能冒充任何用户"的 JWT 小得多 —— 它能碰的只有白名单里那几条路由。
//
// 反过来，Go → ai-service 用的也是同一个密钥（X-Internal-Token），
// 两边的值都从配置注入，源码里不留字面量。
package middleware

import (
	"net/http"
	"strconv"

	"learningplatform/internal/platform/response"
)

// InternalTokenHeader 共享密钥所在的请求头名。
const InternalTokenHeader = "X-Internal-Token"

// InternalUserIDHeader ai-service 转发过来的用户 id（由 Go 代理注入，客户端伪造不到）。
const InternalUserIDHeader = "X-User-Id"

// InternalTokenMiddleware 校验共享密钥。
type InternalTokenMiddleware struct {
	Token string
}

// NewInternalTokenMiddleware 创建中间件。token 为空视为未配置：一律拒绝，
// 绝不能因为"忘了配"就把回调路由变成公开路由。
func NewInternalTokenMiddleware(token string) *InternalTokenMiddleware {
	return &InternalTokenMiddleware{Token: token}
}

// Handle 包一层密钥校验。
func (m *InternalTokenMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if m.Token == "" || r.Header.Get(InternalTokenHeader) != m.Token {
			response.Error(w, &forbiddenError{})
			return
		}
		next(w, r)
	}
}

// forbiddenError 输出 HTTP 403 + code=403。
type forbiddenError struct{}

func (e *forbiddenError) Error() string { return "内部调用凭据无效" }
func (e *forbiddenError) StatusCode() int {
	return http.StatusForbidden
}

// InternalUserID 取出 X-User-Id；缺失或非法时返回 0，由调用方决定怎么报错。
func InternalUserID(r *http.Request) int64 {
	uid, err := strconv.ParseInt(r.Header.Get(InternalUserIDHeader), 10, 64)
	if err != nil || uid <= 0 {
		return 0
	}
	return uid
}
