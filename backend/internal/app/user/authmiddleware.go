package user

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// sessionKeyPrefix Redis 中登录会话的 key 前缀。
// 完整 key = sessionKeyPrefix + token，value 是 SessionUser 的 JSON。
// 登出时按 key 删除即可，不用等 TTL 过期，这就是"Redis 会话方案"的核心。
const sessionKeyPrefix = "login:token:"

// AuthMiddleware 登录鉴权中间件（主流 Redis 会话方案的标准做法）：
// 1. 从请求头 Authorization: Bearer {token} 取出 token
// 2. 用 token 去 Redis 查会话（login:token:{token}）
// 3. 查到 → 把会话信息放进 context，放行到业务逻辑
// 4. 查不到（未登录 / 已登出 / 已过期）→ 直接返回 401
type AuthMiddleware struct {
	svcCtx *svc.ServiceContext
}

// NewAuthMiddleware 创建鉴权中间件。
func NewAuthMiddleware(svcCtx *svc.ServiceContext) *AuthMiddleware {
	return &AuthMiddleware{svcCtx: svcCtx}
}

// Handle 返回中间件处理器。
func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 从 Authorization 头提取 token
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		token = strings.TrimSpace(token)
		if token == "" {
			writeUnauthorized(w, r)
			return
		}

		// 2. 到 Redis 查会话（未登录 / 已登出 / 过期都查不到）
		val, err := m.svcCtx.Redis.Get(sessionKeyPrefix + token)
		if err != nil || val == "" {
			writeUnauthorized(w, r)
			return
		}

		// 3. 解析会话 JSON，放入 context 供后续 handler / logic 使用
		var session model.SessionUser
		if err := json.Unmarshal([]byte(val), &session); err != nil {
			writeUnauthorized(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), authTokenKey, token)
		ctx = context.WithValue(ctx, authSessionKey, session)
		next(w, r.WithContext(ctx))
	}
}

// writeUnauthorized 统一返回 401（未登录或登录已过期）。
func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJsonCtx(r.Context(), w, http.StatusUnauthorized,
		map[string]string{"message": "未登录或登录已过期"})
}
