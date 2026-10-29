package user

// 登录鉴权中间件（主流 Redis 会话方案）。
//
// 1. 从请求头 Authorization: Bearer {token} 取出 token
// 2. 用 token 去 Redis 查会话（login:token:{token}）
// 3. 查到 → 把会话信息放进 context，放行到业务逻辑
// 4. 查不到（未登录 / 已登出 / 已过期）→ 直接返回 401
//
// 另有一个"可选登录"版本 HandleOptional：公开接口（如课程详情）用它，
// 带了有效 token 就识别用户（用于返回 canWatch、个性化数据），
// 没带 token 也照常放行，只是按匿名处理。

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"learningplatform/internal/model"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// SessionKeyPrefix Redis 中登录会话的 key 前缀。
// 完整 key = SessionKeyPrefix + token，value 是 SessionUser 的 JSON。
// 登出时按 key 删除即可，不用等 TTL 过期，这就是"Redis 会话方案"的核心。
const SessionKeyPrefix = "login:token:"

// AuthMiddleware 登录鉴权中间件。
type AuthMiddleware struct {
	svcCtx *svc.ServiceContext
}

// NewAuthMiddleware 创建鉴权中间件。
func NewAuthMiddleware(svcCtx *svc.ServiceContext) *AuthMiddleware {
	return &AuthMiddleware{svcCtx: svcCtx}
}

// Handle 强制登录：没有有效会话直接 401。
func (m *AuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, ok := m.withSession(r)
		if !ok {
			writeUnauthorized(w, r)
			return
		}
		next(w, r.WithContext(ctx))
	}
}

// HandleOptional 可选登录：有 token 就解析，没有/无效也放行。
//
// 只给公开接口用。不要拿它保护写操作 —— 那样未登录请求会一路走到业务层，
// 只是在那里才被拒，攻击面更大。
func (m *AuthMiddleware) HandleOptional(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, _ := m.withSession(r)
		next(w, r.WithContext(ctx))
	}
}

// withSession 解析会话；第二个返回值表示是否登录成功。
// 失败时返回原始 context，供可选登录继续处理。
func (m *AuthMiddleware) withSession(r *http.Request) (context.Context, bool) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	token = strings.TrimSpace(token)
	if token == "" {
		return r.Context(), false
	}

	if m.svcCtx.Redis == nil {
		return r.Context(), false
	}
	val, err := m.svcCtx.Redis.Get(SessionKeyPrefix + token)
	if err != nil || val == "" {
		return r.Context(), false
	}

	var session model.SessionUser
	if err := json.Unmarshal([]byte(val), &session); err != nil {
		return r.Context(), false
	}
	// 角色实时化：会话里缓存的是登录那一刻的角色，
	// 管理员改了角色后，老会话要等 24 小时才生效（期间权限判定全错）。
	// 这里每次请求解析一次角色，用 60 秒的 Redis 缓存挡住数据库压力。
	session.Role = m.resolveRole(session.Username, session.Role)

	ctx := context.WithValue(r.Context(), AuthTokenKey, token)
	ctx = context.WithValue(ctx, AuthSessionKey, session)
	return ctx, true
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
//
// 与其它接口保持同样的 {code,message} 信封：前端拦截器只看状态码，
// 但脚本/SDK 不应该在这里遇到另一种结构。
func writeUnauthorized(w http.ResponseWriter, r *http.Request) {
	response.WriteError(w, &AuthError{Code: http.StatusUnauthorized, Msg: "未登录或登录已过期"})
}
