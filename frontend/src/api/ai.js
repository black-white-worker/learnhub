// AI 模块的前端请求层。
//
// 两条通道：
//   - 普通 JSON 接口走 client（自动带 token、自动解 {code,message,data} 信封）；
//     超时放宽到 150 秒：中文专业出题要先检索中文资料、再调大模型，
//     质量不合格时还会换蓝图重试一轮，实测 50~120 秒。
//   - 答疑与「出题阶段进度」都是 SSE 流，axios 拿不到逐块增量，只能用原生
//     fetch + ReadableStream。代价是绕过了拦截器，所以 token、401、错误文案
//     都在这里自己处理一遍。出题流失败时可回退到上面的普通 JSON 接口。
//
// 超时分层：axios 150s < Go 代理 165s < go-zero 路由 180s < nginx 300s。
// 前端最先超时，才能给老师友好提示，而不是收到 Go 抛的 503。
//
// 后端的对外清单只有 4 条：chat / generate / session/history / session，
// 加上 Go 自己回答的 status。批改入口不在这里 —— 载荷必须由 Go 从数据库组装。

import client, { handleUnauthorized } from './client.js'
import { takeSseBlocks, parseSseBlock, readAiErrorBody } from '../utils/aiSse.js'

const AI_TIMEOUT = 150000

// ===== 状态：前端拿它决定菜单和按钮是否可用 =====
//
// 缓存一份，避免每个页面进来都问一次；配置是进程级的，改一次要重启后端，
// 所以不需要 TTL，只有登录状态可能变化（401 时清掉）。
let statusCache = null

export function aiStatus() {
  return statusCache
}

export async function loadAiStatus(force = false) {
  if (statusCache && !force) return statusCache
  try {
    statusCache = await client.get('/ai/status', { timeout: 10000 })
  } catch {
    // 后端没起来 / 未登录时别让菜单炸掉，按"不可用"处理
    statusCache = { enabled: false, reason: 'AI 服务状态获取失败', features: [] }
  }
  return statusCache
}

export function clearAiStatusCache() {
  statusCache = null
}

// ===== AI 出题 =====
// data: { knowledge, difficulty, qtype, count, score, seed?, exclude_stems?, source?,
//         subject?, grounding?, bank_category? }
// seed / exclude_stems 由 buildGeneratePayload() 生成：每次换一批都换随机种子并排除最近出过的题。
// subject / grounding 控制「按知识点出的简体中文专业题」：服务端检索中文权威条目作命题依据。
// source=bank 是可选的英文通识题库（课堂热身），不按知识点出题。
export function aiGenerate(data) {
  return client.post('/ai/generate', data, { timeout: AI_TIMEOUT })
}

// 同一个出题接口，带 Accept: text/event-stream 时服务端改推阶段进度：
//   event: progress → {stage,status,detail,label,elapsed_ms}
//   event: result   → 与 aiGenerate() 完全一致的结果结构
//   event: error    → {message}
// 中文专业出题要 25~90 秒，流式让页面显示「现在卡在哪一步」而不是一直转圈。
// 流建立失败时调用方回退到 aiGenerate()（老 JSON 链路，行为不变）。
export async function streamAiGenerate({ data, onProgress, signal, fetchImpl } = {}) {
  const doFetch = fetchImpl || ((...args) => globalThis.fetch(...args))
  const token = localStorage.getItem('token')
  const headers = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  if (token) headers.Authorization = 'Bearer ' + token

  const res = await doFetch('/api/ai/generate', {
    method: 'POST',
    headers,
    body: JSON.stringify(data ?? {}),
    signal,
  })

  if (!res.ok) {
    if (res.status === 401) handleUnauthorized()
    let text = ''
    try {
      text = await res.text()
    } catch {
      // 读不到 body 就用状态码兜底
    }
    const err = new Error(readAiErrorBody(text, res.status))
    err.status = res.status
    throw err
  }
  if (!res.body) throw new Error('这个浏览器不支持出题进度流，请换 Chrome/Edge')

  const reader = res.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  let result = null
  let error = ''

  const handle = (block) => {
    const evt = parseSseBlock(block)
    if (evt.event === 'progress' && evt.json) onProgress?.(evt.json)
    else if (evt.event === 'result') result = evt.json
    else if (evt.event === 'error') error = String(evt.json?.message || evt.data || '出题失败')
  }

  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { blocks, rest } = takeSseBlocks(buffer)
      buffer = rest
      for (const block of blocks) handle(block)
    }
    // 最后一块可能不带结尾空行，流断了也得补解析，否则拿不到 result
    buffer += decoder.decode()
    const tail = takeSseBlocks(buffer + '\n\n')
    for (const block of tail.blocks) handle(block)
  } finally {
    reader.cancel().catch(() => {})
  }

  if (error) throw new Error(error)
  if (!result) throw new Error('出题进度流提前结束，没有收到结果')
  return result
}

// ===== 对话历史（按用户 + 课程存，服务端只留最近 5 轮）=====
export function aiSessionHistory(courseId) {
  return client.get('/ai/session/history', { params: { course_id: courseId }, timeout: 10000 })
}

export function aiSessionClear(courseId) {
  return client.delete('/ai/session', { params: { course_id: courseId }, timeout: 10000 })
}

// ===== 主观题 AI 初评（老师侧，走 Go 的后端接口）=====
// base：'/assignment/submission'（作业）或 '/exam/submission'（考试）——
// 两个模块复用同一条 AI 批改链路，只是提交记录存在不同的表里。
export function triggerAiGrade(submissionId, { base = '/assignment/submission' } = {}) {
  return client.post(`${base}/${submissionId}/ai-grade`, {}, { timeout: AI_TIMEOUT })
}

export function getAiGrade(submissionId, { base = '/assignment/submission' } = {}) {
  return client.get(`${base}/${submissionId}/ai-grade`, { timeout: 15000 })
}

// ===== 答疑（SSE）=====
//
// onEvent(evt) 每条事件回调一次，evt = {event, data, json}（见 utils/aiSse）。
// signal 用来做"停止生成"：AbortController.abort() 之后这里抛 AbortError，
// 调用方要区分"用户主动停"和"真的失败了"。
export async function streamAiChat({ query, courseId, sessionId, onEvent, signal, fetchImpl }) {
  const doFetch = fetchImpl || ((...args) => globalThis.fetch(...args))
  const token = localStorage.getItem('token')
  const headers = {
    'Content-Type': 'application/json',
    Accept: 'text/event-stream',
  }
  if (token) headers.Authorization = 'Bearer ' + token

  const res = await doFetch('/api/ai/chat', {
    method: 'POST',
    headers,
    body: JSON.stringify({ query, course_id: courseId, session_id: sessionId ?? null }),
    signal,
  })

  if (!res.ok) {
    if (res.status === 401) handleUnauthorized()
    let text = ''
    try {
      text = await res.text()
    } catch {
      // 读不到 body 就用状态码兜底
    }
    const err = new Error(readAiErrorBody(text, res.status))
    err.status = res.status
    throw err
  }
  if (!res.body) throw new Error('这个浏览器不支持流式回答，请换 Chrome/Edge')

  const reader = res.body.getReader()
  const decoder = new TextDecoder('utf-8')
  let buffer = ''
  try {
    for (;;) {
      const { value, done } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const { blocks, rest } = takeSseBlocks(buffer)
      buffer = rest
      for (const block of blocks) onEvent?.(parseSseBlock(block))
    }
    // 服务端最后一个块可能不带结尾空行，流断了也得补发，不然答案少一句
    buffer += decoder.decode()
    const tail = takeSseBlocks(buffer + '\n\n')
    for (const block of tail.blocks) onEvent?.(parseSseBlock(block))
  } finally {
    reader.cancel().catch(() => {})
  }
}

export default {
  aiStatus,
  loadAiStatus,
  streamAiChat,
  aiGenerate,
  streamAiGenerate,
  aiSessionHistory,
  aiSessionClear,
  triggerAiGrade,
  getAiGrade,
}
