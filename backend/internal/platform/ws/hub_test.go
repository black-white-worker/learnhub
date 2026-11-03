package ws

// Hub 并发模型的单元测试（零外部依赖，可随时运行；推荐带 -race 跑）。
//
// 并发安全性是本模块的核心卖点：Hub 是 clients map 的唯一写者，所有增删改
// 都经 channel 串行化。这些用例只构造最小 Client（不给真实 *websocket.Conn），
// 因为 Hub 在处理 register/unregister/broadcast 时只碰 UserID 与 Send 两个字段，
// 不碰 Conn，因此可以脱离网络层直接验证并发正确性。

import (
	"sync"
	"testing"
	"time"
)

// recvWithTimeout 在超时内从 ch 读出一条消息；超时返回 (nil,false)。
func recvWithTimeout(ch <-chan []byte, timeout time.Duration) ([]byte, bool) {
	select {
	case v, ok := <-ch:
		return v, ok
	case <-time.After(timeout):
		return nil, false
	}
}

// 等待直到 fn 返回 true，或超时（用于等待 Run goroutine 处理完 channel 命令）。
func waitUntil(fn func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fn()
}

// waitClosed 轮询直到 ch 被关闭（用于验证 Unregister 确实 close 了 Send）。
func waitClosed(ch <-chan []byte, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case _, ok := <-ch:
			if !ok {
				return true
			}
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case _, ok := <-ch:
		return !ok
	default:
		return false
	}
}

// 用例 1：注册后 Push，消息能精确送达该用户的 Send 通道。
func TestHub_RegisterAndDeliver(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{UserID: 1, Send: make(chan []byte, 1)}
	hub.Register(client)

	data := []byte(`{"type":"unread","value":1}`)
	hub.Push(1, data)

	got, ok := recvWithTimeout(client.Send, time.Second)
	if !ok {
		t.Fatal("期望注册用户的连接收到推送，但超时未收到")
	}
	if string(got) != string(data) {
		t.Fatalf("推送内容不一致：期望 %s，实际 %s", data, got)
	}
}

// 用例 2：同一用户多条连接，一次 Push 所有连接都收到。
func TestHub_MultipleClientsSameUser(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	c1 := &Client{UserID: 7, Send: make(chan []byte, 1)}
	c2 := &Client{UserID: 7, Send: make(chan []byte, 1)}
	hub.Register(c1)
	hub.Register(c2)

	hub.Push(7, []byte("hello"))

	for i, c := range []*Client{c1, c2} {
		if _, ok := recvWithTimeout(c.Send, time.Second); !ok {
			t.Fatalf("第 %d 条连接未收到广播", i+1)
		}
	}
}

// 用例 3：推送给"没有在线连接"的用户不会 panic，也无需等待。
func TestHub_PushToOfflineUser(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	done := make(chan struct{})
	go func() {
		hub.Push(999, []byte("nobody home"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("推送离线用户时发生阻塞")
	}
}

// 用例 4：注销后，Send 通道被关闭（WritePump 据此退出）。
func TestHub_UnregisterClosesSend(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	client := &Client{UserID: 3, Send: make(chan []byte, 1)}
	hub.Register(client)
	time.Sleep(20 * time.Millisecond) // 等 Run 把 register 处理完，确保后续 unregister 能找到它
	hub.Unregister(client)

	// 等 Run 处理完 unregister（会 close(c.Send)）
	if !waitClosed(client.Send, time.Second) {
		t.Fatal("注销后 Send 通道未被关闭")
	}
}

// 用例 5：Push 永远非阻塞——队列满且没有消费者时直接丢弃，绝不卡住调用方。
// 这是"业务侧推送不能阻塞 API 请求"这一设计约束的直接证明。
func TestHub_PushNeverBlocks(t *testing.T) {
	hub := NewHub()
	// 故意不启动 Run：broadcast 缓冲 256 满后无人消费

	// 先填满缓冲（256 条）
	for i := 0; i < 256; i++ {
		hub.Push(1, []byte("fill"))
	}
	if len(hub.broadcast) != 256 {
		t.Fatalf("期望广播缓冲满 256，实际 %d", len(hub.broadcast))
	}

	// 第 257 条：select-default 分支，必须立即返回
	done := make(chan struct{})
	go func() {
		hub.Push(1, []byte("overflow"))
		close(done)
	}()
	select {
	case <-done:
		// ok
	case <-time.After(time.Second):
		t.Fatal("队列满时 Push 发生了阻塞（应当直接丢弃返回）")
	}
	// 多出来的那条走了 default，没有进入缓冲
	if len(hub.broadcast) != 256 {
		t.Fatalf("溢出消息不应进入缓冲，期望仍 256，实际 %d", len(hub.broadcast))
	}
}

// 用例 6：并发压力——多 goroutine 同时 Push / 注册 / 注销，配合 Run 跑，
// 验证 map 在单写者模型下不会出现 data race（-race 能抓到）。
func TestHub_ConcurrentAccess(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	var wg sync.WaitGroup
	// 10 个 goroutine 疯狂推送
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				hub.Push(int64(base+i%5), []byte("x"))
			}
		}(g * 100)
	}
	// 1 个 goroutine 反复注册/注销同一个用户的一堆连接
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			c := &Client{UserID: 1, Send: make(chan []byte, 1)}
			hub.Register(c)
			hub.Push(1, []byte("y"))
			hub.Unregister(c)
		}
	}()
	wg.Wait()
	// 能跑到这里且 -race 无告警，即证明并发安全。
}
