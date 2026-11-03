// 实时通知长连接的单元测试（Node 内置测试运行器，零依赖）。
//
// 用可控的 MockWebSocket 替代浏览器原生 WebSocket，验证：
//  1. 带 token 时拼出正确的 ws:// 地址（http→ws）；
//  2. 无 token 时不建立连接；
//  3. https 页面自动切到 wss://；
//  4. 收到 unread / announcement 消息正确分发到回调；
//  5. 非法 JSON 静默忽略；
//  6. 连接被服务端断开后按延迟自动重连；
//  7. 主动 disconnect 能阻止重连。

import { test } from 'node:test'
import assert from 'node:assert/strict'

const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

// 可控的 WebSocket 替身：记录实例、可手动触发消息/关闭/错误。
function makeMockWebSocket() {
  const instances = []
  class MockWebSocket {
    constructor(url) {
      this.url = url
      this.onmessage = null
      this.onclose = null
      this.onerror = null
      this.readyState = 1 // OPEN
      instances.push(this)
    }
    close() {
      if (this.readyState === 3) return
      this.readyState = 3 // CLOSED
      if (this.onclose) this.onclose({})
    }
    // 模拟服务端推送一条消息
    _send(data) {
      if (this.onmessage) this.onmessage({ data })
    }
    // 模拟连接错误（会触发 onerror → close → onclose）
    _error() {
      if (this.onerror) this.onerror({})
    }
  }
  return { MockWebSocket, instances }
}

test('带 token 时拼出正确的 ws 地址（http→ws）', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'abc', protocol: 'http:', host: 'app.test' })
  socket.connect()
  assert.equal(instances.length, 1)
  assert.equal(instances[0].url, 'ws://app.test/ws/notifications?token=abc')
  socket.disconnect()
})

test('无 token 时不建立任何连接', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: '' })
  socket.connect()
  assert.equal(instances.length, 0)
  socket.disconnect()
})

test('https 页面自动切到 wss', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x', protocol: 'https:', host: 'app.test' })
  socket.connect()
  assert.equal(instances[0].url, 'wss://app.test/ws/notifications?token=x')
  socket.disconnect()
})

test('收到 unread 消息 → 调用 onUnread 回调', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const events = []
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x', onUnread: (m) => events.push(['unread', m]) })
  socket.connect()
  instances[0]._send('{"type":"unread","value":3}')
  assert.equal(events.length, 1)
  assert.deepEqual(events[0], ['unread', { type: 'unread', value: 3 }])
  socket.disconnect()
})

test('收到 announcement 消息 → 调用 onAnnouncement 回调', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const events = []
  const socket = makeNotificationSocketForTest(MockWebSocket, {
    token: 'x',
    onAnnouncement: (m) => events.push(['ann', m]),
  })
  socket.connect()
  instances[0]._send('{"type":"announcement","id":99,"title":"新公告"}')
  assert.equal(events.length, 1)
  assert.equal(events[0][1].id, 99)
  socket.disconnect()
})

test('非法 JSON 静默忽略，不触发任何回调', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const events = []
  const socket = makeNotificationSocketForTest(MockWebSocket, {
    token: 'x',
    onUnread: () => events.push('u'),
    onAnnouncement: () => events.push('a'),
  })
  socket.connect()
  instances[0]._send('not-json{')
  assert.equal(events.length, 0)
  socket.disconnect()
})

test('连接被断开后按延迟自动重连', async () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x', reconnectDelay: 30 })
  socket.connect()
  assert.equal(instances.length, 1)
  // 模拟服务端把连接关了
  instances[0].close()
  // 等重连定时器（30ms）触发
  await sleep(80)
  assert.equal(instances.length, 2, '应当在断线后重建一条连接')
  socket.disconnect()
})

test('主动 disconnect 能阻止断线后的重连', async () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x', reconnectDelay: 1000 })
  socket.connect()
  assert.equal(instances.length, 1)
  instances[0].close() // 服务端断开
  socket.disconnect() // 主动断开：清掉重连定时器
  await sleep(50)
  assert.equal(instances.length, 1, 'disconnect 后不应再重连')
})

test('主动 disconnect 后再次 connect 能重新连上（登录态切换场景）', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x' })
  socket.connect()
  assert.equal(instances.length, 1)
  socket.disconnect()
  // App.vue 在 username 变化时会 disconnect(); connect()，
  // 如果 connect 不重置内部状态，重新登录后就再也收不到实时通知。
  socket.connect()
  assert.equal(instances.length, 2, 'disconnect 后再次 connect 应当重建连接')
  socket.disconnect()
})

test('重复 connect 不会叠出多条连接', () => {
  const { MockWebSocket, instances } = makeMockWebSocket()
  const socket = makeNotificationSocketForTest(MockWebSocket, { token: 'x' })
  socket.connect()
  socket.connect()
  assert.equal(instances.length, 1, '已连接时重复 connect 应复用原连接')
  socket.disconnect()
})

// 测试辅助：用 mock 装配出 createNotificationSocket 实例。
// 这样既能注入 MockWebSocket，又能注入回调与 location，不必依赖浏览器全局。
import { createNotificationSocket } from '../src/utils/notificationSocket.js'

function makeNotificationSocketForTest(MockWebSocket, opts) {
  return createNotificationSocket({
    getToken: () => opts.token,
    location: { protocol: opts.protocol || 'http:', host: opts.host || 'localhost' },
    WebSocketImpl: MockWebSocket,
    onUnread: opts.onUnread,
    onAnnouncement: opts.onAnnouncement,
    reconnectDelay: opts.reconnectDelay ?? 5000,
  })
}
