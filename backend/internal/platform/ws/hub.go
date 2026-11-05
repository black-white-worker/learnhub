// Package ws 实时通知的 WebSocket 基础设施。
//
// 并发模型（核心）：
//   - Hub 是 clients map 的唯一拥有者，所有增删改都通过 channel 串行化，
//     由唯一一个 Run() goroutine 处理，因此 map 无需加锁也能保证并发安全。
//   - 每个连接由 ReadPump / WritePump 两个 goroutine 驱动，并带 ping/pong 心跳。
//   - 业务侧调用 Push 是非阻塞的（select + default），不会拖慢 HTTP 请求。
package ws

import "github.com/gorilla/websocket"

// Message 定向推送给某用户的消息。
type Message struct {
	UserID int64
	Data   []byte
}

// Client 表示一个用户的单条 WebSocket 连接。
type Client struct {
	Hub    *Hub
	UserID int64
	Conn   *websocket.Conn
	Send   chan []byte // 由 WritePump 消费
}

// Hub 管理所有在线连接。clients[userID] 是该用户的多条连接集合。
// 它是 clients map 的唯一写者，所有变更都通过 channel 递交。
type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan *Message // 缓冲 256，降低 Push 因队列满而丢弃的概率
	clients    map[int64]map[*Client]bool
}

// NewHub 构造 Hub。
func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan *Message, 256),
		clients:    make(map[int64]map[*Client]bool),
	}
}

// Run 必须在独立的 goroutine 中运行（go hub.Run()）。
// 它是 clients map 的唯一写者，通过 select 串行处理所有命令，天然并发安全。
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			set := h.clients[c.UserID]
			if set == nil {
				set = make(map[*Client]bool)
				h.clients[c.UserID] = set
			}
			set[c] = true

		case c := <-h.unregister:
			if set, ok := h.clients[c.UserID]; ok {
				if _, ok := set[c]; ok {
					delete(set, c)
					close(c.Send) // 关闭后 WritePump 的 range 会退出
					if len(set) == 0 {
						delete(h.clients, c.UserID)
					}
				}
			}

		case m := <-h.broadcast:
			if set, ok := h.clients[m.UserID]; ok {
				for c := range set {
					select {
					case c.Send <- m.Data:
					default:
						// 该连接积压，直接摘掉，避免拖慢整次广播
						close(c.Send)
						delete(set, c)
					}
				}
			}
		}
	}
}

// Register 把新连接交给 Hub（非阻塞递交）。
func (h *Hub) Register(c *Client) { h.register <- c }

// Unregister 把断开的连接从 Hub 摘除。
func (h *Hub) Unregister(c *Client) { h.unregister <- c }

// Push 非阻塞地向某用户推送消息：队列满则丢弃。
// 实时通知允许偶发丢失，前端有 60s 轮询兜底保证最终一致。
func (h *Hub) Push(userID int64, data []byte) {
	select {
	case h.broadcast <- &Message{UserID: userID, Data: data}:
	default:
	}
}
