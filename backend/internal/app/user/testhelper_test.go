package user

// 用户模块测试的公共环境准备。
//
// 中间件用例只需要 Redis（会话），不需要 MySQL：
// 连不上 Redis 就整体跳过，而不是让 go test ./... 一片红。

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// newTestSvcCtx 构造一个只依赖 Redis 的 ServiceContext。
// MySQL 连接是懒加载的，这里不实际连接。
func newTestSvcCtx() *svc.ServiceContext {
	c := config.Config{}
	c.Redis.Type = "node"
	c.Redis.Host = "127.0.0.1:6381"
	c.Redis.Pass = "learningplatform_redis_2026"
	return svc.NewServiceContext(c)
}

// TestMain 依赖探测：Redis 连得上才跑，否则整体跳过。
func TestMain(m *testing.M) {
	if !redisReady() {
		fmt.Println("[skip] 鉴权中间件测试需要 Redis(6381)，未检测到依赖，跳过。")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func redisReady() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6381", 800*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
