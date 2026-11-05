package handler

// /ws/notifications 握手 + 端到端推送的集成测试（前后端联调的后端半场）。
//
// 覆盖三条关键路径：
//  1. 不带 token → 401 拒绝；
//  2. 带无效 token（Redis 查不到会话）→ 401 拒绝；
//  3. 带有效 token → 升级成功，服务端 NotifHub.Push 的消息能实时到达客户端。
//
// 依赖 Redis（会话存储）：127.0.0.1:6381 不可达时整包跳过，避免 CI 红。

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/config"
	"learningplatform/internal/svc"

	"github.com/gorilla/websocket"
	"strconv"
)

func newWsTestSvcCtx(t *testing.T) *svc.ServiceContext {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:6381", 800*time.Millisecond)
	if err != nil {
		t.Skipf("Redis(127.0.0.1:6381) 不可达，跳过 WS 握手集成测试: %v", err)
	}
	_ = conn.Close()

	c := config.Config{}
	c.Redis.Type = "node"
	c.Redis.Host = "127.0.0.1:6381"
	c.Redis.Pass = os.Getenv("TEST_REDIS_PASS")
	return svc.NewServiceContext(c)
}

// 用例 1：无 token → 401。
func TestWSHandler_NoToken(t *testing.T) {
	svcCtx := newWsTestSvcCtx(t)
	ts := httptest.NewServer(http.HandlerFunc(wsNotificationsHandler(svcCtx)))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws/notifications")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", resp.StatusCode)
	}
}

// 用例 2：无效 token（Redis 无会话）→ 401。
func TestWSHandler_InvalidToken(t *testing.T) {
	svcCtx := newWsTestSvcCtx(t)
	ts := httptest.NewServer(http.HandlerFunc(wsNotificationsHandler(svcCtx)))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ws/notifications?token=not-a-real-token")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("期望 401，实际 %d", resp.StatusCode)
	}
}

// 用例 3：有效 token → 升级成功，且服务端 Push 能实时送达。
func TestWSHandler_ValidTokenAndPush(t *testing.T) {
	svcCtx := newWsTestSvcCtx(t)

	const token = "ws-integration-token"
	const userID = int64(424242)
	// 预置会话（等价于登录成功后写入 Redis 的状态）
	if err := svcCtx.Redis.Setex(user.SessionKeyPrefix+token, `{"id":`+strconv.FormatInt(userID, 10)+`}`, 3600); err != nil {
		t.Fatalf("预置 Redis 会话失败: %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(wsNotificationsHandler(svcCtx)))
	defer ts.Close()

	wsURL := "ws" + ts.URL[len("http"):] + "/ws/notifications?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("WebSocket 握手失败: %v", err)
	}
	defer conn.Close()

	// 等 Hub 把 register 命令处理完，再推送
	time.Sleep(100 * time.Millisecond)
	payload := []byte(`{"type":"unread","value":1}`)
	svcCtx.NotifHub.Push(userID, payload)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("读取推送消息失败（实时推送链路不通）: %v", err)
	}
	if string(msg) != string(payload) {
		t.Fatalf("推送内容不一致：期望 %s，实际 %s", payload, msg)
	}
}
