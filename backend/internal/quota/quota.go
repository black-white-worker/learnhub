// Package quota 上传配额（按用户每日字节限额）。
//
// 背景：上传接口此前只做了"谁能传"（课程创建者），没有"能传多少"。
// 一个老师账号被盗，或者学生被误授课程权限，就可以把 MinIO 灌满。
//
// 实现：用 Redis 按 (用户, 日期) 计数，写入时先"预占"（INCRBY），
// 超限则回滚并拒绝。选择 Redis 而不是 MySQL 的原因：
//   - 天然带 TTL，第二天自动清零，不需要定时任务；
//   - 计数是热路径，不值得为它落一张表、加一次事务。
//
// 已知取舍：预占发生在解析文件之前，若上传中途失败，额度会被多占，
// 直到次日归零。对"防灌满"这个目标来说是可接受的（宁可少传，不可多传）。
package quota

import (
	"fmt"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// keyTTL 配额 key 的存活时间。48 小时保证跨时区/跨日边界不会提前丢计数。
const keyTTL = 48 * time.Hour

// Allowed 纯函数：已用 used、本次 add、上限 limit，是否还放得下。
//
// limit <= 0 表示不限制（配置未开启），永远放行；
// 这样默认部署行为与改造前完全一致，不会因为忘配而挡住上传。
func Allowed(used, add, limit int64) bool {
	if limit <= 0 {
		return true
	}
	if add < 0 {
		add = 0
	}
	if used < 0 {
		used = 0
	}
	return used+add <= limit
}

// QuotaKey 生成当日配额 key：upload:quota:20261027:42
func QuotaKey(userId int64, now time.Time) string {
	return fmt.Sprintf("upload:quota:%s:%d", now.Format("20060102"), userId)
}

// Exceeded 超限错误。实现 StatusCode()，response 包会输出 413。
type Exceeded struct {
	Limit int64
	Used  int64
	Add   int64
}

func (e *Exceeded) Error() string {
	return fmt.Sprintf("今日上传额度不足：已用 %.1f MB / 上限 %.1f MB，本次需要 %.1f MB",
		mb(e.Used), mb(e.Limit), mb(e.Add))
}

// StatusCode 413 Payload Too Large —— 语义上就是"你要传的东西超出允许范围"。
func (e *Exceeded) StatusCode() int { return 413 }

func mb(b int64) float64 { return float64(b) / (1 << 20) }

// Limiter 配额限制器。零值/nil 都安全：不限制。
type Limiter struct {
	rdb   *redis.Redis
	limit int64
}

// New 创建限制器；limit <= 0 时整个功能关闭（Enabled() == false）。
func New(rdb *redis.Redis, limit int64) *Limiter {
	return &Limiter{rdb: rdb, limit: limit}
}

// Enabled 是否真的在限制。Redis 不可用时也不限制（可用性优先）。
func (l *Limiter) Enabled() bool {
	return l != nil && l.limit > 0 && l.rdb != nil
}

// Limit 返回当前上限（字节），0 表示不限制。
func (l *Limiter) Limit() int64 {
	if l == nil {
		return 0
	}
	return l.limit
}

// Used 查询今日已用字节数（读失败按 0 处理）。
func (l *Limiter) Used(userId int64) int64 {
	if !l.Enabled() {
		return 0
	}
	key := QuotaKey(userId, time.Now())
	v, err := l.rdb.Get(key)
	if err != nil || v == "" {
		return 0
	}
	var n int64
	_, _ = fmt.Sscan(v, &n)
	return n
}

// Reserve 预占额度：成功返回 nil；超限返回 *Exceeded 并回滚。
//
// 用 INCRBY 而不是"先读后写"，是为了让并发上传也不会一起挤过上限。
func (l *Limiter) Reserve(userId, add int64) error {
	if !l.Enabled() || userId <= 0 {
		return nil
	}
	if add < 0 {
		add = 0
	}
	key := QuotaKey(userId, time.Now())

	n, err := l.rdb.Incrby(key, add)
	if err != nil {
		// Redis 抖动不应阻断上传：配额是防护，不是主流程
		return nil
	}
	if n == add {
		// 首次写入本日计数，补上 TTL
		_ = l.rdb.Expire(key, int(keyTTL.Seconds()))
	}
	if n > l.limit {
		// 回滚，避免"一次超限把当天额度吃光"
		_, _ = l.rdb.Decrby(key, add)
		return &Exceeded{Limit: l.limit, Used: l.Used(userId), Add: add}
	}
	return nil
}
