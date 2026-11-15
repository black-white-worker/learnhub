// 出题阶段进度：纯函数归约 + SSE 请求层。
// 全链路不碰真实网络：fetch 用注入的假实现，流用 ReadableStream 造。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import './support/dom-shim.mjs'

import { streamAiGenerate } from '../src/api/ai.js'
import {
  GENERATE_STAGES,
  generationProgressSummary,
  newGenerateProgress,
  progressPercent,
  reduceGenerateProgress,
} from '../src/utils/aiGenerate.js'

localStorage.setItem('token', 'tok-progress')

function sseResponse(chunks, { status = 200 } = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    body: new ReadableStream({
      start(controller) {
        for (const ch of chunks) controller.enqueue(new TextEncoder().encode(ch))
        controller.close()
      },
    }),
    async text() {
      return chunks.join('')
    },
  }
}

function progressEvent(stage, status, detail, elapsed_ms = 0) {
  return `event: progress\ndata: ${JSON.stringify({ stage, status, detail, elapsed_ms })}\n\n`
}

const RESULT = {
  questions: [{ stem: '光子能量大于禁带宽度时会发生什么？', answer: 'A', options: ['甲', '乙', '丙', '丁'] }],
  qtype: 'single', count: 1, invalid_count: 0, discarded_count: 0, meta: { seed: 's1', attempts: 1 },
}

// ===== 纯函数：阶段状态机 =====

test('阶段事件按顺序推进，done 之后所有阶段收尾', () => {
  let s = newGenerateProgress()
  assert.equal(s.stages.length, GENERATE_STAGES.length)
  assert.equal(progressPercent(s), 0)

  s = reduceGenerateProgress(s, { stage: 'prepare', status: 'running', detail: '3 道单选题', elapsed_ms: 10 })
  assert.equal(s.active, 'prepare')
  assert.equal(s.stages[0].status, 'running')
  assert.equal(s.stages[0].detail, '3 道单选题')
  assert.ok(progressPercent(s) > 0)

  s = reduceGenerateProgress(s, { stage: 'prepare', status: 'done', detail: '已分配角度' })
  s = reduceGenerateProgress(s, { stage: 'materials', status: 'running', detail: '检索中', elapsed_ms: 800 })
  assert.equal(s.stages[0].status, 'done')
  assert.equal(s.active, 'materials')

  s = reduceGenerateProgress(s, { stage: 'materials', status: 'skipped', detail: '资料源不可达' })
  s = reduceGenerateProgress(s, { stage: 'compose', status: 'running', detail: '第 1/2 轮生成中' })
  s = reduceGenerateProgress(s, { stage: 'verify', status: 'done', detail: '合格 3 题' })
  assert.equal(s.stages[1].status, 'skipped')
  assert.equal(s.stages[3].detail, '合格 3 题')

  s = reduceGenerateProgress(s, { stage: 'done', status: 'done', detail: '共 3 题', count: 3, elapsed_ms: 26000 })
  assert.equal(s.done, true)
  assert.equal(s.active, '')
  assert.equal(s.summary, '共 3 题')
  assert.equal(s.lastElapsed, 26000)
  assert.equal(progressPercent(s), 100)
  // skipped 的阶段在收尾时不能被误标成「已完成」
  assert.equal(s.stages[1].status, 'skipped')
  assert.equal(s.stages[2].status, 'done')
})

test('重试时已完成的阶段会被重新激活，未知阶段不破坏状态', () => {
  let s = newGenerateProgress()
  s = reduceGenerateProgress(s, { stage: 'compose', status: 'running', detail: '第 1 轮' })
  s = reduceGenerateProgress(s, { stage: 'verify', status: 'done', detail: '全废' })
  s = reduceGenerateProgress(s, { stage: 'compose', status: 'running', detail: '第 2 轮', label: '重新命制' })
  assert.equal(s.stages[2].status, 'running')
  assert.equal(s.stages[2].label, '重新命制')
  assert.equal(s.stages[3].status, 'done')

  const before = JSON.stringify(s.stages)
  s = reduceGenerateProgress(s, { stage: 'future_stage', status: 'running', detail: 'x' })
  assert.equal(JSON.stringify(s.stages), before)
  assert.equal(s.done, false)
})

test('进度条按完成阶段推进并永远不假装 100%', () => {
  let s = newGenerateProgress()
  s = reduceGenerateProgress(s, { stage: 'prepare', status: 'running', detail: '' })
  const p1 = progressPercent(s)
  s = reduceGenerateProgress(s, { stage: 'prepare', status: 'done', detail: '' })
  const p2 = progressPercent(s)
  assert.ok(p2 > p1)
  assert.ok(p2 < 100)
  assert.ok(progressPercent(s) <= 95)
})

test('完成摘要包含耗时、资料、轮次与重出/排重', () => {
  const text = generationProgressSummary(28, {
    materials: [{ title: '光电效应' }, { title: '内光电效应' }],
    attempts: 2, discarded_count: 1, deduped: 1,
  })
  assert.match(text, /耗时 28 秒/)
  assert.match(text, /引用 2 段资料/)
  assert.match(text, /2 轮命制/)
  assert.match(text, /重出 1 题/)
  assert.match(text, /排重 1 题/)
})

// ===== 请求层：SSE 解析 =====

test('streamAiGenerate 带 Accept 并逐条派发进度、返回 result', async () => {
  const seen = []
  const calls = []
  const chunks = [
    ': connected\n\n',
    progressEvent('prepare', 'running', '3 道单选题', 12),
    ': keep-alive\n\n',
    progressEvent('compose', 'running', '第 1/2 轮生成中', 1500),
    `event: result\ndata: ${JSON.stringify(RESULT)}\n\n`,
  ]
  const data = await streamAiGenerate({
    data: { knowledge: '光电效应' },
    onProgress: (e) => seen.push(e),
    fetchImpl: async (url, opts) => {
      calls.push({ url, opts })
      return sseResponse(chunks)
    },
  })
  assert.deepEqual(data, RESULT)
  assert.equal(calls[0].url, '/api/ai/generate')
  assert.match(calls[0].opts.headers.Accept, /text\/event-stream/)
  assert.equal(JSON.parse(calls[0].opts.body).knowledge, '光电效应')
  assert.deepEqual(seen.map((e) => e.stage), ['prepare', 'compose'])
})

test('最后一块不带结尾空行也能拿到 result', async () => {
  const chunks = [
    progressEvent('prepare', 'done', '完成'),
    `event: result\ndata: ${JSON.stringify(RESULT)}`,
  ]
  const data = await streamAiGenerate({ fetchImpl: async () => sseResponse(chunks) })
  assert.equal(data.count, 1)
})

test('流内 error 事件与提前结束都会抛错，便于上层回退或提示', async () => {
  await assert.rejects(
    () => streamAiGenerate({
      fetchImpl: async () => sseResponse([
        progressEvent('compose', 'running', '生成中'),
        'event: error\ndata: {"message":"AI 出题服务暂不可用"}\n\n',
      ]),
    }),
    /AI 出题服务暂不可用/,
  )
  await assert.rejects(
    () => streamAiGenerate({ fetchImpl: async () => sseResponse([progressEvent('prepare', 'done', 'ok')]) }),
    /没有收到结果/,
  )
})

test('HTTP 层失败按状态码转成可读错误', async () => {
  await assert.rejects(
    () => streamAiGenerate({
      fetchImpl: async () => sseResponse(['{"message":"服务不可用"}'], { status: 503 }),
    }),
    (e) => e.status === 503 && /服务不可用|AI 服务暂时不可用/.test(e.message),
  )
})
