package logic

// 登录失败限流（防撞库 / 暴力猜密码）。
//
// 背景：登录接口此前没有任何频率限制，可以无限次试密码。
// 这是整个系统最容易被扫的入口，成本最低的防护就是"按 IP + 用户名计数"。
//
// 规则：同一 (IP, 用户名) 连续失败 5 次 → 锁定 10 分钟，期间直接拒绝；
// 登录成功清零。计数放 Redis（10 分钟 TTL，自然过期），失败不阻塞主流程。
//
// 为什么 key 里带用户名：只按 IP 会把整个机房的 NAT 出口连坐；
// 只按用户名则会让攻击者用一个 IP 打不同账号时不受限。两者拼接更精确。

import (
	"fmt"
	"strings"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

const (
	// LoginFailKeyPrefix 登录失败计数 key 前缀：login:fail:{ip}:{username}
	LoginFailKeyPrefix = "login:fail:"
	// LoginMaxFails 允许的连续失败次数
	LoginMaxFails = 5
	// LoginLockSeconds 超过次数后的锁定时长（秒）
	LoginLockSeconds = 600
)

// LoginFailKey 生成失败计数 key。用户名统一小写，避免大小写绕过。
func LoginFailKey(ip, username string) string {
	u := strings.ToLower(strings.TrimSpace(username))
	i := strings.TrimSpace(ip)
	if i == "" {
		i = "unknown"
	}
	return fmt.Sprintf("%s%s:%s", LoginFailKeyPrefix, i, u)
}

// LoginLocked 纯函数：失败次数是否已达上限。
func LoginLocked(fails int) bool { return fails >= LoginMaxFails }

// checkLoginLock 登录前检查：已锁定则返回 true（调用方直接拒绝）。
// Redis 不可用时返回 false —— 限流是防护，不该在存储抖动时把所有人挡在门外。
func checkLoginLock(rdb *redis.Redis, ip, username string) bool {
	if rdb == nil {
		return false
	}
	v, err := rdb.Get(LoginFailKey(ip, username))
	if err != nil || v == "" {
		return false
	}
	var n int
	if _, err := fmt.Sscan(v, &n); err != nil {
		return false
	}
	return LoginLocked(n)
}

// recordLoginFail 记录一次失败：INCRBY + 首次写入时补 TTL。
func recordLoginFail(rdb *redis.Redis, ip, username string) {
	if rdb == nil {
		return
	}
	key := LoginFailKey(ip, username)
	n, err := rdb.Incr(key)
	if err != nil {
		return
	}
	if n == 1 {
		_ = rdb.Expire(key, LoginLockSeconds)
	}
}

// clearLoginFail 登录成功后清零计数。
func clearLoginFail(rdb *redis.Redis, ip, username string) {
	if rdb == nil {
		return
	}
	_, _ = rdb.Del(LoginFailKey(ip, username))
}

// LoginGuard 便于测试/复用的组合结构（保持与 svc 解耦）。
type LoginGuard struct {
	rdb *redis.Redis
}

// NewLoginGuard 创建登录限流守卫。
func NewLoginGuard(rdb *redis.Redis) *LoginGuard { return &LoginGuard{rdb: rdb} }

// Locked 当前是否已锁定。
func (g *LoginGuard) Locked(ip, username string) bool {
	if g == nil {
		return false
	}
	return checkLoginLock(g.rdb, ip, username)
}

// Fail 记一次失败。
func (g *LoginGuard) Fail(ip, username string) {
	if g == nil {
		return
	}
	recordLoginFail(g.rdb, ip, username)
}

// Success 清零。
func (g *LoginGuard) Success(ip, username string) {
	if g == nil {
		return
	}
	clearLoginFail(g.rdb, ip, username)
}

// loginGuardOf 从 svcCtx 取 Redis 构造守卫（svcCtx 可能为 nil，测试友好）。
func loginGuardOf(svcCtx *svc.ServiceContext) *LoginGuard {
	if svcCtx == nil {
		return nil
	}
	return NewLoginGuard(svcCtx.Redis)
}
