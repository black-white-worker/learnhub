// 出题随机性载荷 / 元数据展示的单测：确保「换一批」真的换种子且排除旧题。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  GEN_SOURCES,
  buildGeneratePayload,
  collectStems,
  generationMetaText,
  newGenerationSeed,
  questionExplanation,
  questionFocus,
  toAddQuestionPayload,
  payloadProblem,
} from '../src/utils/aiGenerate.js'

const form = {
  knowledge: '光电效应', difficulty: '中等', qtype: 'single', count: 3, score: 5,
}

test('来源下拉只有 AI 出题与免费题库两项', () => {
  assert.deepEqual(GEN_SOURCES.map((s) => s.value), ['ai', 'bank'])
})

test('newGenerationSeed 每次都不同且非空', () => {
  const seeds = Array.from({ length: 20 }, () => newGenerationSeed())
  assert.equal(new Set(seeds).size, 20)
  assert.ok(seeds.every((s) => typeof s === 'string' && s.length >= 8))
})

test('buildGeneratePayload：默认 AI 出题、带随机种子、字段完整', () => {
  const p = buildGeneratePayload(form, { seed: 'fixed-seed' })
  assert.equal(p.source, 'ai')
  assert.equal(p.seed, 'fixed-seed')
  assert.equal(p.knowledge, '光电效应')
  assert.equal(p.count, 3)
  assert.equal(p.score, 5)
  assert.deepEqual(p.exclude_stems, [])
})

test('buildGeneratePayload：没传 seed 时自动换，且两次请求不同', () => {
  const a = buildGeneratePayload(form)
  const b = buildGeneratePayload(form)
  assert.notEqual(a.seed, b.seed)
})

test('buildGeneratePayload：上一批题干去重、去空、最多 20 条', () => {
  const stems = ['  题干A  ', '', '题干A', ...Array.from({ length: 25 }, (_, i) => `题${i}`)]
  const p = buildGeneratePayload(form, { seed: 's', previousStems: stems })
  assert.equal(p.exclude_stems.length, 20)
  assert.equal(p.exclude_stems[0], '题干A')
  assert.equal(new Set(p.exclude_stems).size, 20)
})

test('buildGeneratePayload：题库模式允许空知识点、count/score 有边界', () => {
  const p = buildGeneratePayload({ ...form, knowledge: '', count: 99, score: -3, source: 'bank' },
    { seed: 'bankseed' })
  assert.equal(p.source, 'bank')
  assert.equal(p.knowledge, '')
  assert.equal(p.count, 10)
  assert.equal(p.score, 1)
})

test('collectStems 累积最近几批题干，去重且不超上限', () => {
  const batch1 = collectStems([{ stem: '题1' }, { stem: '题2' }], [])
  const batch2 = collectStems([{ stem: '题2' }, { stem: '题3' }], batch1)
  assert.deepEqual(batch2, ['题2', '题3', '题1'])
  const many = collectStems(Array.from({ length: 30 }, (_, i) => ({ stem: `题${i}` })), [], 20)
  assert.equal(many.length, 20)
})

test('generationMetaText：随机角度 / 来源 / 排重 / 回退都看得见', () => {
  const text = generationMetaText({
    seed: 'abcdef123456', source: 'ai', strategy: ['定量计算', '实验现象', '易错陷阱'],
    deduped: 2, attempts: 2,
  })
  assert.match(text, /批次 abcdef12/)
  assert.match(text, /角度 定量计算 \/ 实验现象 \/ 易错陷阱/)
  assert.match(text, /排重 2 题/)
  assert.match(text, /补齐 2 轮/)

  assert.match(generationMetaText({ source: 'bank', bank_provider: 'opentdb', localized: true }),
    /Open Trivia DB·已汉化/)
  assert.match(generationMetaText({ source: 'bank', strategy: ['Politics', 'Art'] }), /分类 Politics \/ Art/)
  assert.match(generationMetaText({ source: 'ai', bank_status: 'fallback: timeout' }),
    /已回退 AI 出题/)
  assert.equal(generationMetaText(null), '')
})

test('解析与考查点兼容中文键名', () => {
  assert.equal(questionExplanation({ 解析: '因为频率低于截止频率' }), '因为频率低于截止频率')
  assert.equal(questionFocus({ 考点: '截止频率' }), '截止频率')
  assert.equal(questionExplanation({}), '')
})

test('题库模式返回的 qtype 覆盖表单 qtype，入库不会把单选题当多选提交', () => {
  // 表单选多选，但服务端题库只有单选 → 页面用服务端 qtype=single 转换
  const payload = toAddQuestionPayload(
    { stem: 'Which planet is red?', options: ['Mars', 'Venus', 'Jupiter', 'Mercury'], answer: 'A', score: 5 },
    'single'
  )
  assert.equal(payload.type, 'single')
  assert.equal(payloadProblem(payload), null)
})
