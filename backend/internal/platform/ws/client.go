package ws

import (
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second    // 写超时：单次写操作最长等待时间
	pongWait       = 60 * time.Second    // 读超时：超过未收到任何消息视为断开
	pingPeriod     = (pongWait * 9) / 10 // 心跳周期，略小于 pongWait 留余量
	maxMessageSize = 512                 // 单帧最大字节，前端只收不发，值可很小
)

// ReadPump 跑在请求 goroutine 上：读取对端消息并检测心跳超时。
// 前端一般只收不发，这里忽略收到的内容；一旦断开或超时即退出，
// 由 defer 负责从 Hub 注销并关闭连接。
func (c *Client) ReadPump() {
	defer func() {
		c.Hub.Unregister(c)
		c.Conn.Close()
	}()
	c.Conn.SetReadLimit(maxMessageSize)
	c.Conn.SetReadDeadline(time.Now().Add(pongWait))
	c.Conn.SetPongHandler(func(string) error {
		c.Conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		if _, _, err := c.Conn.ReadMessage(); err != nil {
			break
		}
	}
}

// WritePump 跑在独立 goroutine 上：把 Send channel 里的消息写出，并定时发 ping 心跳。
// 当 Hub 关闭 c.Send 后，for-range 自然退出，连接被回收。
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.Send:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok { // Hub 已关闭 Send（连接已被注销）
				c.Conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.Conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.Conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
