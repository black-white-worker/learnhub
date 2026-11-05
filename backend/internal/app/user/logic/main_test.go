package logic

// 集成测试依赖探测：MySQL / Redis 不可用、或未提供连接信息时整包跳过，而不是让 CI 红。
//
// 说明：本包用例复用 loginlogic_test.go 里的 newTestSvcCtx（真连 MySQL+Redis）。
// 连接信息一律从环境变量读取，不写死在仓库里：
//
//	TEST_MYSQL_DSN    形如 root:<密码>@tcp(127.0.0.1:3316)/learningplatform?charset=utf8mb4&parseTime=true&loc=Local
//	TEST_REDIS_PASS   Redis 密码（无密码时也要显式设为空串）
//
// 注意：一个包只能有一个 TestMain，所以这里的判定同时覆盖 main_test.go 与
// loginlogic_test.go 两边的需求（环境变量 + 端口探测），不要在其他 *_test.go 里再声明。

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if ok, reason := testDepsReady(); !ok {
		fmt.Println("[skip] 用户逻辑集成测试需要 MySQL(3316) 与 Redis(6381)：" + reason)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// testDepsReady 先看环境变量是否配好，再探测端口是否真的通。
// 只探测端口不检查环境变量会产生更迷惑的失败：依赖在跑、但 newTestSvcCtx 拿到空 DSN。
func testDepsReady() (bool, string) {
	if os.Getenv("TEST_MYSQL_DSN") == "" {
		return false, "未设置 TEST_MYSQL_DSN"
	}
	if _, ok := os.LookupEnv("TEST_REDIS_PASS"); !ok {
		return false, "未设置 TEST_REDIS_PASS（Redis 无密码时也要显式设为空串）"
	}
	for _, addr := range []string{"127.0.0.1:3316", "127.0.0.1:6381"} {
		conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
		if err != nil {
			return false, "连不上 " + addr + "（" + err.Error() + "）"
		}
		_ = conn.Close()
	}
	return true, ""
}
