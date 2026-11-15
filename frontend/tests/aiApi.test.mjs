// AI 请求层单测：SSE 读取、错误文案、status 缓存、以及每个 JSON 接口的地址和参数。
//
// 全程不碰真实网络：fetch 用注入的假实现，axios 换成 adapter 桩。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import './support/dom-shim.mjs'

import client from '../src/api/client.js'
import {
  aiGenerate,
  aiSessionClear,
  aiSessionHistory,
  clearAiStatusCache,
  getAiGrade,
  loadAiStatus,
  streamAiChat,
  triggerAiGrade,
} from '../src/api/ai.js'
import { newChatState, reduceChatEvent } from '../src/utils/aiSse.js'

localStorage.setItem('token', 'tok-123')

// ===== 假 fetch =====
function sseResponse(chunks) {
  const stream = new ReadableStream({
    start(controller) {
      for (const ch of chunks) controller.enqueue(new TextEncoder().encode(ch))
      controller.close()
    },
  })
  return { ok: true, status: 200, body: stream, text: async () => '' }
}

function errorResponse(status, bodyText) {
  return { ok: false, status, body: null, text: async () => bodyText }
}

function recordFetch(result) {
  const calls = []
  const impl = async (url, init) => {
    calls.push({ url, init })
    if (result instanceof Error) throw result
    return result
  }
  return { calls, impl }
}

// ===== streamAiChat =====
test('streamAiChat 用 POST /api/ai/chat + 可信 token + 蛇形参数', async () => {
  const { calls, impl } = recordFetch(sseResponse(['data: {"delta":"好"}\n\n']))
  await streamAiChat({ query: '你好', courseId: 7, fetchImpl: impl, onEvent: () => {} })

  const [call] = calls
  assert.equal(call.url, '/api/ai/chat')
  assert.equal(call.init.method, 'POST')
  assert.equal(call.init.headers.Authorization, 'Bearer tok-123')
  assert.equal(call.init.headers.Accept, 'text/event-stream')
  const body = JSON.parse(call.init.body)
  assert.deepEqual(body, { query: '你好', course_id: 7, session_id: null })
})

test('streamAiChat 把 delta / refs / done 依次交给 onEvent', async () => {
  const chunks = [
    'event: refs\ndata: ["第一章"]\n\n',
    'data: {"delta":"概率"}\n\n',
    'data: {"delta":"是"}\n\n',
    'data: {"done":true,"answer":"概率是"}\n\n',
  ]
  const { impl } = recordFetch(sseResponse(chunks))
  const state = newChatState()
  await streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl, onEvent: (e) => reduceChatEvent(state, e) })

  assert.equal(state.answer, '概率是')
  assert.deepEqual(state.refs, ['第一章'])
  assert.equal(state.done, true)
})

test('最后一块没有结尾空行也要补发，否则答案少一句', async () => {
  const { impl } = recordFetch(sseResponse(['data: {"delta":"尾句没换行"}']))
  const seen = []
  await streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl, onEvent: (e) => seen.push(e) })
  assert.equal(seen.length, 1)
  assert.equal(seen[0].json.delta, '尾句没换行')
})

test('块被网络切成两半也不会丢事件', async () => {
  const { impl } = recordFetch(sseResponse(['data: {"del', 'ta":"拼回来"}\n\n']))
  const state = newChatState()
  await streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl, onEvent: (e) => reduceChatEvent(state, e) })
  assert.equal(state.answer, '拼回来')
})

test('429 读 detail 并带上状态码，前端要按"额度用完"提示', async () => {
  const { impl } = recordFetch(errorResponse(429, '{"detail":"今日 AI 调用已达上限"}'))
  await assert.rejects(
    () => streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl }),
    (err) => {
      assert.equal(err.message, '今日 AI 调用已达上限')
      assert.equal(err.status, 429)
      return true
    }
  )
})

test('流内 error 事件不会抛异常，但会被累加器记下来', async () => {
  const { impl } = recordFetch(sseResponse(['data: {"error":"模型超时"}\n\n']))
  const state = newChatState()
  await streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl, onEvent: (e) => reduceChatEvent(state, e) })
  assert.equal(state.error, '模型超时')
})

test('signal 会原样透传，停止生成才真的能中止', async () => {
  const ctrl = new AbortController()
  const { calls, impl } = recordFetch(sseResponse(['data: {"done":true}\n\n']))
  await streamAiChat({ query: 'q', courseId: 1, fetchImpl: impl, signal: ctrl.signal, onEvent: () => {} })
  assert.equal(calls[0].init.signal, ctrl.signal)
})

// ===== JSON 接口 =====
// axios 的 adapter 换成桩：既能断言最终 URL/params，又不会真的发请求。
let lastConfig = null
function stubAdapter(data) {
  const original = client.defaults.adapter
  client.defaults.adapter = async (config) => {
    lastConfig = config
    return { data, status: 200, statusText: 'OK', headers: {}, config }
  }
  return () => {
    client.defaults.adapter = original
  }
}

function fullUrl(config) {
  const params = config.params
    ? '?' +
      Object.entries(config.params)
        .map(([k, v]) => `${k}=${v}`)
        .join('&')
    : ''
  return `${config.baseURL}${config.url}${params}`
}

test('aiGenerate 走 /api/ai/generate，超时放宽到 150 秒', async () => {
  const restore = stubAdapter({ questions: [] })
  try {
    await aiGenerate({ knowledge: '假设检验', qtype: 'single', count: 2, score: 5 })
    assert.equal(fullUrl(lastConfig), '/api/ai/generate')
    assert.equal(lastConfig.timeout, 150000)
  } finally {
    restore()
  }
})

test('对话历史用 course_id 查询参数（Python 侧是蛇形）', async () => {
  const restore = stubAdapter({ messages: [] })
  try {
    await aiSessionHistory(12)
    assert.equal(fullUrl(lastConfig), '/api/ai/session/history?course_id=12')
    assert.equal(lastConfig.method, 'get')
  } finally {
    restore()
  }
})

test('清空历史是 DELETE /api/ai/session?course_id=', async () => {
  const restore = stubAdapter({ deleted: true })
  try {
    await aiSessionClear(12)
    assert.equal(fullUrl(lastConfig), '/api/ai/session?course_id=12')
    assert.equal(lastConfig.method, 'delete')
  } finally {
    restore()
  }
})

test('批改走的是 Go 的作业接口，不是 ai-service 的 /api/ai/grade', async () => {
  const restore = stubAdapter({ message: '已提交', grade: null })
  try {
    await triggerAiGrade(88)
    assert.equal(fullUrl(lastConfig), '/api/assignment/submission/88/ai-grade')
    assert.equal(lastConfig.method, 'post')
  } finally {
    restore()
  }

  const restore2 = stubAdapter({ submissionId: 88, status: 'done', score: 12, maxScore: 20, items: [] })
  try {
    await getAiGrade(88)
    assert.equal(fullUrl(lastConfig), '/api/assignment/submission/88/ai-grade')
    assert.equal(lastConfig.method, 'get')
  } finally {
    restore2()
  }
})

// ===== status 缓存 =====
test('loadAiStatus 命中缓存就不重复请求', async () => {
  clearAiStatusCache()
  let hits = 0
  const original = client.defaults.adapter
  client.defaults.adapter = async (config) => {
    hits++
    lastConfig = config
    return { data: { enabled: true, reason: '', features: ['chat'] }, status: 200, statusText: 'OK', headers: {}, config }
  }
  try {
    await loadAiStatus()
    await loadAiStatus()
    assert.equal(hits, 1)
    assert.equal(fullUrl(lastConfig), '/api/ai/status')
  } finally {
    client.defaults.adapter = original
    clearAiStatusCache()
  }
})

test('status 请求失败时按未启用处理，菜单不会崩', async () => {
  clearAiStatusCache()
  const original = client.defaults.adapter
  client.defaults.adapter = async (config) => {
    throw Object.assign(new Error('boom'), { config, response: { status: 500, data: {} } })
  }
  try {
    const s = await loadAiStatus(true)
    assert.equal(s.enabled, false)
    assert.ok(s.reason)
  } finally {
    client.defaults.adapter = original
    clearAiStatusCache()
  }
})
