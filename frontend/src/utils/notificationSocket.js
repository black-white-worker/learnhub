// 实时通知长连接管理（从 App.vue 抽出，便于在 node 下单测）。
//
// 职责边界：本模块只负责"连哪个地址、怎么解析消息、断了怎么重连、主动怎么断开"，
// 不碰任何 UI 状态——收到消息后通过回调交给调用方（App.vue 去改未读数、拉一次列表）。
//
// 与设计文档一致：
//  - 浏览器 WebSocket 握手无法带自定义请求头，token 走 query 参数；
//  - 协议随页面自动切换（http→ws / https→wss）；
//  - 断线后延迟重连；未连接期间靠页面加载/刷新时的 loadUnread 兜底。

export function createNotificationSocket({
  getToken,
  location = globalThis.location,
  WebSocketImpl = globalThis.WebSocket,
  onUnread,
  onAnnouncement,
  reconnectDelay = 5000,
} = {}) {
  let ws = null
  let reconnectTimer = null
  let intentionallyClosed = false

  // 根据当前登录 token 拼出 ws/wss 地址；没有 token 返回 null（不连）。
  function buildUrl() {
    const token = getToken && getToken()
    if (!token) return null
    const proto = location && location.protocol === 'https:' ? 'wss' : 'ws'
    return `${proto}://${location.host}/ws/notifications?token=${token}`
  }

  // 解析单条推送并分发到对应回调；非法 JSON 静默忽略（与设计一致）。
  function handleMessage(event) {
    let msg
    try {
      msg = JSON.parse(event.data)
    } catch {
      return
    }
    if (!msg || typeof msg !== 'object') return
    if (msg.type === 'unread') {
      if (onUnread) onUnread(msg)
    } else if (msg.type === 'announcement') {
      if (onAnnouncement) onAnnouncement(msg)
    }
  }

  // 建立一条新连接。只由 connect 调用，保证同一时刻最多一条连接。
  function open() {
    const url = buildUrl()
    if (!url) return // 没 token 时不建立连接

    const sock = new WebSocketImpl(url)
    ws = sock
    sock.onmessage = handleMessage
    sock.onclose = () => {
      // 只处理"当前这条"连接的关闭，避免旧连接的迟到事件把新连接置空
      if (ws === sock) ws = null
      // 断线后延迟重连；重连期间靠 loadUnread 兜底最终一致
      if (intentionallyClosed) return
      reconnectTimer = setTimeout(connect, reconnectDelay)
    }
    sock.onerror = () => {
      if (ws === sock) sock.close()
    }
  }

  // connect 可重复调用：已有连接则复用；主动断开（登录态变化）后再次调用会重新连上。
  function connect() {
    intentionallyClosed = false
    if (ws) return
    open()
  }

  // 主动断开：标记意图并清掉重连定时器，避免退出登录/卸载时又重连。
  // 注意：这只是"停止当前连接"，之后显式调用 connect() 仍能恢复。
  function disconnect() {
    intentionallyClosed = true
    if (reconnectTimer) clearTimeout(reconnectTimer)
    reconnectTimer = null
    if (ws) {
      const sock = ws
      ws = null
      sock.onclose = null // 主动断开不触发重连
      sock.close()
    }
  }

  return { connect, disconnect, _handleMessage: handleMessage, _buildUrl: buildUrl }
}
