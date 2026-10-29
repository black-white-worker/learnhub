package logic

// 集成测试依赖探测：MySQL / Redis 不可用时整包跳过，而不是让 CI 红。
//
// 说明：本包用例复用 loginlogic_test.go 里的 newTestSvcCtx（真连 MySQL+Redis）。
// 如果本机没起 docker compose，直接跑 go test ./... 会一片 FAIL，
// 用 TestMain 统一判定，保持 "本地跳过、CI 起服务后真跑" 的行为。

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	if !testDepsReady() {
		fmt.Println("[skip] 用户逻辑集成测试需要 MySQL(3316) 与 Redis(6381)，未检测到依赖，跳过。")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func testDepsReady() bool {
	for _, addr := range []string{"127.0.0.1:3316", "127.0.0.1:6381"} {
		conn, err := net.DialTimeout("tcp", addr, 800*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
	}
	return true
}
