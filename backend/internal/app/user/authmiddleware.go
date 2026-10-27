package user

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// SessionKeyPrefix Redis 中登录会话的 key 前缀。
// 完整 key = SessionKeyPrefix + token，value 是 SessionUser 的 JSON。
// 登出时按 key 删除即可，不用等 TTL 过期，这就是"Redis 会话方案"的核心。
const SessionKeyPrefix = "login:token:"

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
		val, err := m.svcCtx.Redis.Get(SessionKeyPrefix + token)
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
		// 3.1 角色实时化：会话里缓存的是登录那一刻的角色，
		// 管理员改了角色后，老会话要等 24 小时才生效（期间权限判定全错）。
		// 这里每次请求解析一次角色，用 60 秒的 Redis 缓存挡住数据库压力。
		session.Role = m.resolveRole(session.Username, session.Role)
		ctx := context.WithValue(r.Context(), AuthTokenKey, token)
		ctx = context.WithValue(ctx, AuthSessionKey, session)
		next(w, r.WithContext(ctx))
	}
}

// RoleCacheKeyPrefix 用户角色的短缓存前缀（与 24h 的登录会话区分开）。
const RoleCacheKeyPrefix = "user:role:"

// roleCacheTTL 角色缓存有效期（秒）。管理员改角色后最多 60 秒内生效；
// 若同时调用了 InvalidateRoleCache，则立即生效。
const roleCacheTTL = 60

// resolveRole 取用户当前角色：先看 60 秒缓存，再回源数据库。
//
// 降级策略：数据库或 Redis 出错时返回会话里的旧角色，
// 保证"存储抖动"不会把所有人踢成未登录。
func (m *AuthMiddleware) resolveRole(username, fallback string) string {
	if username == "" {
		return fallback
	}
	key := RoleCacheKeyPrefix + username
	if m.svcCtx.Redis != nil {
		if v, err := m.svcCtx.Redis.Get(key); err == nil && v != "" {
			return v
		}
	}
	if u := m.svcCtx.UserStore.GetUser(username); u != nil && u.Role != "" {
		if m.svcCtx.Redis != nil {
			_ = m.svcCtx.Redis.Setex(key, u.Role, roleCacheTTL)
		}
		return u.Role
	}
	return fallback
}

// InvalidateRoleCache 角色变更后立即失效缓存（管理员改角色、审批老师时调用）。
func InvalidateRoleCache(rdb *redis.Redis, username string) {
	if rdb == nil || username == "" {
		return
	}
	_, _ = rdb.Del(RoleCacheKeyPrefix + username)
}

// writeUnauthorized 统一返回 401（未登录或登录已过期）。
func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJsonCtx(r.Context(), w, http.StatusUnauthorized,
		map[string]string{"message": "未登录或登录已过期"})
}
