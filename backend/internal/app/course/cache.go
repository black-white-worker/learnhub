package course

// 课程列表缓存的失效策略。
//
// 问题：改造前缓存 key 是 course:list:p1:z20，TTL 60 秒，
// 但创建/编辑/删除课程后不会清理缓存——老师新建的课自己在 60 秒内都看不到。
//
// 做法：给缓存加一个"版本号"计数器 course:list:ver。
// 读的时候把版本号拼进 key，写操作后 INCR 版本号，旧 key 立刻不再被命中，
// 自然过期淘汰。相比 SCAN + DEL 模式串，这条路是 O(1)，且不会阻塞 Redis。

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
)

// CourseListVersionKey 课程列表缓存的版本号 key。
const CourseListVersionKey = "course:list:ver"

// ListCacheVersion 读当前缓存版本号；Redis 不可用或没设置时返回 "0"。
// 返回字符串是为了直接拼 key，避免 int 转换。
func ListCacheVersion(rdb *redis.Redis) string {
	if rdb == nil {
		return "0"
	}
	v, err := rdb.Get(CourseListVersionKey)
	if err != nil || v == "" {
		return "0"
	}
	return v
}

// InvalidateListCache 课程数据变更后调用：版本号 +1，让旧缓存立即失效。
// 失败只影响"缓存多存活最多 60 秒"，不影响数据正确性，所以不返回错误。
func InvalidateListCache(rdb *redis.Redis) {
	if rdb == nil {
		return
	}
	_, _ = rdb.Incr(CourseListVersionKey)
}
