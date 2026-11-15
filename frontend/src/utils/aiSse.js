// AI 答疑 SSE 的纯解析逻辑（从 api/ai.js 抽出来，方便脱离浏览器单测）。
//
// ai-service 那边的事件格式（app/graph/rag_graph.py 的 stream_answer）：
//   event: refs                     → data: ["片段标题", ...]      （可能没有）
//   data: {"delta": "文字"}          → 逐 token 追加
//   data: {"done": true, "answer":} → 收尾
//   data: {"error": "文字"}          → 流内报错（HTTP 状态还是 200）
// HTTP 层的错（429 超额度 / 403 没权限 / 503 没配置）不走事件，是普通响应体。

// takeSseBlocks 从缓冲区里切出"已经完整"的事件块。
// 完整块的定义：以空行结尾。所以最后一段没空行的一定要留在 rest 里等下次，
// 否则会切出半截 JSON。
export function takeSseBlocks(buffer) {
  const norm = String(buffer ?? '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const parts = norm.split('\n\n')
  const rest = parts.pop() ?? ''
  return { blocks: parts.filter((b) => b.trim() !== ''), rest }
}

// parseSseBlock 把一块文本解析成 {event, data, json}。
// 多条 data: 行按 SSE 规范用 \n 连接；json 解析失败时为 null（原始文本仍在 data 里）。
export function parseSseBlock(block) {
  const lines = String(block ?? '').split('\n')
  let event = 'message'
  const dataLines = []
  for (const line of lines) {
    if (line.startsWith(':')) continue // 注释 / 心跳
    if (line.startsWith('event:')) {
      event = line.slice(6).trim() || 'message'
    } else if (line.startsWith('data:')) {
      dataLines.push(line.slice(5).replace(/^ /, ''))
    }
    // id: / retry: 这里用不上，忽略
  }
  const data = dataLines.join('\n')
  let json = null
  if (data) {
    try {
      json = JSON.parse(data)
    } catch {
      json = null
    }
  }
  return { event, data, json }
}

// newChatState 答疑流的累加器。
export function newChatState() {
  return { answer: '', refs: [], error: '', done: false }
}

// reduceChatEvent 把一条事件并进取答态，返回同一个对象（调用方按引用渲染）。
// 未知事件直接忽略：以后 Python 侧加新事件不能把前端聊死。
export function reduceChatEvent(state, evt) {
  if (!evt) return state
  const payload = evt.json || {}
  // refs 既可能是 `event: refs` + 裸数组，也可能是 {"refs": [...]}
  const refList = Array.isArray(evt.json) ? evt.json : payload.refs
  if (Array.isArray(refList)) {
    for (const r of refList) {
      const title = String(r ?? '').trim()
      if (title && !state.refs.includes(title)) state.refs.push(title)
    }
    return state
  }
  if (typeof payload.error === 'string' && payload.error) {
    state.error = payload.error
    return state
  }
  if (typeof payload.delta === 'string') {
    state.answer += payload.delta
    return state
  }
  if (payload.done) {
    state.done = true
    // 收尾带上完整答案时以它为准（中途累加万一漏了块也能被纠正）
    if (typeof payload.answer === 'string' && payload.answer.length >= state.answer.length) {
      state.answer = payload.answer
    }
    return state
  }
  // 只在"这段 data 根本不是 JSON"时才当纯文本追加；
  // 否则 {"heartbeat":{}} 这类未知事件会把 JSON 字面量吐进答案里。
  if (evt.json === null && evt.data) {
    state.answer += evt.data
  }
  return state
}

// readAiError 从 HTTP 错误响应里掏出人话。
// 兼容三种：ai-service 的 {"detail": "..."}（FastAPI）、Go 的 {"code,message}、纯文本。
export function readAiErrorBody(text, status) {
  const raw = String(text ?? '').trim()
  if (raw) {
    try {
      const obj = JSON.parse(raw)
      const msg = obj?.detail || obj?.message || obj?.msg
      if (typeof msg === 'string' && msg.trim()) return msg.trim()
    } catch {
      return raw.length > 200 ? raw.slice(0, 200) + '…' : raw
    }
  }
  return AI_STATUS_TEXT[status] || `请求失败（${status}）`
}

// 状态码兜底文案：额度/权限/未配置这三种是最常碰到的，给明确原因而不是"失败了"。
export const AI_STATUS_TEXT = {
  401: '登录已过期，请重新登录',
  403: '这个 AI 功能只有老师或管理员能用',
  429: '今天的 AI 调用次数已用完，明天再来',
  503: 'AI 服务暂时不可用（未配置或正在重启），请稍后重试',
  504: 'AI 想太久了，稍后重试一次',
}

export function aiStatusText(status) {
  return AI_STATUS_TEXT[status] || ''
}
