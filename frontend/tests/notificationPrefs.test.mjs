// 通知偏好 API 层的集成测试（Node 内置测试运行器，零新增依赖）。
//
// 不依赖浏览器 / 实验性模块 mock：直接把真实 rawClient 的 baseURL 指到本地起的
// HTTP 服务器，验证 fetchNotificationPrefs / updateNotificationPrefs 发了正确的请求
// （方法、路径、body），并按 rawClient"不解包"的约定返回原始响应。
// 这样连请求拦截器（自动带 token）也一并测到了。

import { test } from 'node:test'
import assert from 'node:assert/strict'
import http from 'node:http'

// 浏览器全局 shim：client.js 的请求拦截器会读 localStorage.getItem('token')。
if (typeof globalThis.localStorage === 'undefined') {
  const store = new Map()
  globalThis.localStorage = {
    getItem: (k) => (store.has(k) ? store.get(k) : null),
    setItem: (k, v) => store.set(k, String(v)),
    removeItem: (k) => store.delete(k),
    clear: () => store.clear(),
  }
}

// 本地桩服务器：记录收到的请求，并按设计回一个统一格式响应。
const received = { method: null, url: null, body: null }
const server = http.createServer((req, res) => {
  received.method = req.method
  // Node 有时会把 req.url 报告成绝对地址（absolute-form 请求目标），统一取 pathname。
  received.url = req.url.startsWith('http')
    ? new URL(req.url).pathname
    : req.url
  if (req.method === 'POST') {
    let buf = ''
    req.on('data', (c) => (buf += c))
    req.on('end', () => {
      received.body = buf
      res.setHeader('Content-Type', 'application/json')
      res.end(JSON.stringify({ code: 0, message: 'ok', data: { message: 'ok' } }))
    })
    return
  }
  res.setHeader('Content-Type', 'application/json')
  res.end(JSON.stringify({ code: 0, message: 'ok', data: { prefs: [{ channel: 'inapp', enabled: 1 }] } }))
})
await new Promise((r) => server.listen(0, r))
const base = `http://127.0.0.1:${server.address().port}`

// 把真实 rawClient 指到本地服务器（index.js 复用的是同一个实例）。
// 保留 /api 前缀，使完整请求地址与生产一致：base/api + /notification-prefs。
const { rawClient } = await import('../src/api/client.js')
rawClient.defaults.baseURL = base + '/api'

const { fetchNotificationPrefs, updateNotificationPrefs } = await import('../src/api/index.js')

test('fetchNotificationPrefs 走 GET /api/notification-prefs', async () => {
  const res = await fetchNotificationPrefs()
  assert.equal(received.method, 'GET')
  assert.equal(received.url, '/api/notification-prefs')
  // rawClient 不解包信封，返回的是完整响应；ProfileView 取 res.data.data.prefs
  assert.equal(res.data.data.prefs[0].channel, 'inapp')
})

test('updateNotificationPrefs 走 POST /api/notification-prefs 并带上 body', async () => {
  const res = await updateNotificationPrefs({ channel: 'email', enabled: 0 })
  assert.equal(received.method, 'POST')
  assert.equal(received.url, '/api/notification-prefs')
  assert.deepEqual(JSON.parse(received.body), { channel: 'email', enabled: 0 })
  assert.equal(res.data.data.message, 'ok')
})

test('teardown: 关闭本地服务器', () => {
  server.close()
})
