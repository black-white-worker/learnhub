// 中文专业题链路的前端纯函数单测：学科 / 中文资料开关 / 依据与素材元信息展示。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  GEN_SOURCES,
  GEN_SUBJECTS,
  buildGeneratePayload,
  generationMetaText,
  questionBasis,
} from '../src/utils/aiGenerate.js'

const baseForm = {
  knowledge: '爱因斯坦光电方程与截止频率',
  difficulty: '中等',
  qtype: 'single',
  count: 3,
  score: 5,
  source: 'ai',
}

test('默认来源是按知识点的中文专业题，bank 只是可选通识热身', () => {
  assert.equal(GEN_SOURCES[0].value, 'ai')
  assert.match(GEN_SOURCES[0].label, /中文专业题/)
  assert.match(GEN_SOURCES[1].label, /通识热身/)
  assert.equal(GEN_SUBJECTS[0].value, '')
  assert.ok(GEN_SUBJECTS.some((s) => s.value === '物理学'))
})

test('出题载荷带上学科语境与中文资料增强开关', () => {
  const payload = buildGeneratePayload(
    { ...baseForm, subject: '物理学', grounding: true },
    { seed: 'fixed-seed', previousStems: ['上一批题干'] },
  )
  assert.equal(payload.subject, '物理学')
  assert.equal(payload.grounding, true)
  assert.equal(payload.knowledge, '爱因斯坦光电方程与截止频率')
  assert.deepEqual(payload.exclude_stems, ['上一批题干'])
  assert.equal(payload.seed, 'fixed-seed')
})

test('关掉资料增强 / 不填学科时载荷仍合法，学科超长会截断', () => {
  const off = buildGeneratePayload({ ...baseForm, grounding: false }, { seed: 's' })
  assert.equal(off.grounding, false)
  assert.equal(off.subject, '')
  const long = buildGeneratePayload({ ...baseForm, subject: '物'.repeat(50) }, { seed: 's' })
  assert.equal(long.subject.length, 24)
})

test('通识题库模式仍可显式选择，且不误传学科语境', () => {
  const payload = buildGeneratePayload(
    { ...baseForm, source: 'bank', knowledge: '', subject: '物理学' },
    { seed: 's' },
  )
  assert.equal(payload.source, 'bank')
  assert.equal(payload.knowledge, '')
  assert.equal(payload.subject, '物理学') // 服务端在 bank 模式下忽略
})

test('元信息如实展示中文资料命中 / 降级 / 关闭三种状态', () => {
  const ok = generationMetaText({
    seed: 'abcd1234', source: 'ai', grounding: true,
    material_status: 'ok', subject: '物理学',
    materials: [{ title: '光电效应' }, { title: '内光电效应' }],
    strategy: ['定量计算'],
  })
  assert.match(ok, /中文资料 2 条/)
  assert.match(ok, /物理学/)
  assert.match(ok, /角度 定量计算/)

  assert.match(generationMetaText({ source: 'ai', grounding: true, material_status: 'unavailable' }),
    /资料源不可用/)
  assert.match(generationMetaText({ source: 'ai', grounding: true, material_status: 'no_match' }),
    /未检索到资料/)
  assert.match(generationMetaText({ source: 'ai', grounding: true, material_status: 'disabled' }),
    /未引用资料/)
  assert.doesNotMatch(generationMetaText({ source: 'ai', grounding: false, material_status: 'disabled' }),
    /未引用资料/)
})

test('题库模式的元信息不会被中文资料状态污染', () => {
  const text = generationMetaText({
    seed: 'bank1', source: 'bank', bank_provider: 'opentdb', localized: true,
    strategy: ['Astronomy'], material_status: 'disabled', grounding: false,
  })
  assert.match(text, /分类 Astronomy/)
  assert.match(text, /Open Trivia DB·已汉化/)
  assert.doesNotMatch(text, /中文资料|资料源/)
})

test('补齐成功时只展示「自动重出 N 题」，不误报成题目不合格', () => {
  const text = generationMetaText({ source: 'ai', discarded_count: 2, attempts: 2 })
  assert.match(text, /自动重出 2 题/)
  assert.match(text, /补齐 2 轮/)
  assert.doesNotMatch(generationMetaText({ source: 'ai', attempts: 1 }), /自动重出/)
})

test('命题依据兼容 basis / 中文键名，空值返回空串', () => {
  assert.equal(questionBasis({ basis: '依据[1]：爱因斯坦光电方程' }), '依据[1]：爱因斯坦光电方程')
  assert.equal(questionBasis({ 依据: '依据[2]：截止频率' }), '依据[2]：截止频率')
  assert.equal(questionBasis({}), '')
  assert.equal(questionBasis(null), '')
})
