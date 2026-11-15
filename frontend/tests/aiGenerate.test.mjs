// AI 出题结果 → 平台题目结构的转换单测。
// 盯的是三处最容易写错的地方：判断题的布尔/字符串、选择题的数组/JSON、多选答案的顺序。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  GEN_TYPES,
  answerLabel,
  isQuestionUsable,
  normalizeJudgeAnswer,
  normalizeMultiAnswer,
  normalizeGeneratedQuestions,
  normalizeOptions,
  normalizeQuestionShape,
  optionLines,
  payloadProblem,
  toAddQuestionPayload,
} from '../src/utils/aiGenerate.js'

test('题型只暴露作业页能渲染的 4 种（没有 fill）', () => {
  assert.deepEqual(
    GEN_TYPES.map((t) => t.value),
    ['single', 'multiple', 'judge', 'essay']
  )
})

test('判断题：AI 给布尔，平台要字符串 true/false', () => {
  assert.equal(normalizeJudgeAnswer(true), 'true')
  assert.equal(normalizeJudgeAnswer(false), 'false')
  assert.equal(normalizeJudgeAnswer('对'), 'true')
  assert.equal(normalizeJudgeAnswer('错误'), 'false')
  assert.equal(normalizeJudgeAnswer('TRUE'), 'true')
  assert.equal(normalizeJudgeAnswer(undefined), '')
})

test('多选题答案统一成排序后的逗号字母串（平台按 "A,C" 判分）', () => {
  assert.equal(normalizeMultiAnswer(['D', 'A', 'B']), 'A,B,D')
  assert.equal(normalizeMultiAnswer('DBA'), 'A,B,D')
  assert.equal(normalizeMultiAnswer('A,C'), 'A,C')
  assert.equal(normalizeMultiAnswer(''), '')
})

test('单选入库载荷：选项变 JSON 字符串，答案保持字母', () => {
  const p = toAddQuestionPayload(
    { stem: ' 1+1=? ', options: ['0', '1', '2', '3'], answer: 'C', score: 5 },
    'single'
  )
  assert.deepEqual(p, {
    type: 'single',
    stem: '1+1=?',
    options: '["0","1","2","3"]',
    answer: 'C',
    score: 5,
  })
  assert.equal(payloadProblem(p), null)
})

test('判断题入库载荷不会带 options', () => {
  const p = toAddQuestionPayload({ stem: '地球是方的', answer: false, score: 3 }, 'judge')
  assert.equal(p.options, '')
  assert.equal(p.answer, 'false')
  assert.equal(p.score, 3)
  assert.equal(payloadProblem(p), null)
})

test('主观题参考答案保留原文', () => {
  const p = toAddQuestionPayload({ stem: '简述贝叶斯定理', answer: 'P(A|B)=P(B|A)P(A)/P(B)', score: 10 }, 'essay')
  assert.equal(p.answer, 'P(A|B)=P(B|A)P(A)/P(B)')
  assert.equal(p.options, '')
})

test('score 缺失或为 0 时回落到 5 分', () => {
  assert.equal(toAddQuestionPayload({ stem: 'x', answer: 'A' }, 'single').score, 5)
  assert.equal(toAddQuestionPayload({ stem: 'x', answer: 'A', score: 0 }, 'single').score, 5)
})

test('payloadProblem 拦住"答案结构不合格"的题', () => {
  assert.equal(payloadProblem({ type: 'single', stem: '缺选项', options: '', answer: 'A' }), '缺少选项')
  assert.equal(payloadProblem({ type: 'single', stem: '答案越界', options: '["a","b","c","d"]', answer: 'E' }), '单选答案要写成 A-D 之一')
  assert.equal(payloadProblem({ type: 'multiple', stem: '只一个答案', options: '["a","b","c","d"]', answer: 'A' }), '多选答案至少两个字母')
  assert.equal(payloadProblem({ type: 'essay', stem: '没参考答案', options: '', answer: '' }), '主观题没有参考答案，AI 批改时无法对照')
  assert.equal(payloadProblem({ type: 'judge', stem: '答案不是对错', options: '', answer: '也许' }), '判断题答案必须是 对/错')
  assert.equal(payloadProblem({ type: 'single', stem: '', options: '["a","b","c","d"]', answer: 'A' }), '题干为空')
})

test('服务端标了 invalid 的题不可选', () => {
  assert.equal(isQuestionUsable({ stem: '题干', invalid: true }), false)
  assert.equal(isQuestionUsable({ stem: '   ' }), false)
  assert.equal(isQuestionUsable({ stem: '正常题' }), true)
  assert.equal(isQuestionUsable(null), false)
})

test('answerLabel：选择题把答案字母换成选项原文', () => {
  const q = { stem: 's', options: ['猫', '狗', '鱼', '鸟'], answer: 'BD' }
  assert.equal(answerLabel(q, 'multiple'), 'B. 狗　D. 鸟')
  // 平台入库后的形式是 "A,C"，显示时同样要认
  assert.equal(answerLabel({ options: ['猫', '狗', '鱼', '鸟'], answer: 'A,C' }, 'multiple'), 'A. 猫　C. 鱼')
  assert.equal(answerLabel({ options: ['猫', '狗'], answer: 'A' }, 'single'), 'A. 猫')
})

test('answerLabel：判断题显示 对/错', () => {
  assert.equal(answerLabel({ answer: true }, 'judge'), '对')
  assert.equal(answerLabel({ answer: '错' }, 'judge'), '错')
})

test('optionLines 给出 A/B/C/D 前缀', () => {
  assert.deepEqual(optionLines({ options: ['甲', '乙'] }), [
    { letter: 'A', text: '甲' },
    { letter: 'B', text: '乙' },
  ])
  assert.deepEqual(optionLines({}), [])
})

test('normalizeOptions：JSON 字符串 / 对象 / 带前缀数组都能还原成数组', () => {
  assert.deepEqual(normalizeOptions('["猫","狗","鱼","鸟"]'), ['猫', '狗', '鱼', '鸟'])
  assert.deepEqual(normalizeOptions({ A: '猫', B: '狗', C: '鱼', D: '鸟' }), ['猫', '狗', '鱼', '鸟'])
  assert.deepEqual(normalizeOptions(['A. 猫', 'B. 狗', 'C. 鱼', 'D. 鸟']), ['猫', '狗', '鱼', '鸟'])
  assert.deepEqual(normalizeOptions([{ text: '猫' }, { label: '狗' }]), ['猫', '狗'])
  assert.deepEqual(normalizeOptions('猫,狗,鱼,鸟'), ['猫', '狗', '鱼', '鸟'])
  assert.deepEqual(normalizeOptions(null), [])
})

test('normalizeQuestionShape：中文键名 + 连写答案，题干和选项都不再丢', () => {
  const q = normalizeQuestionShape(
    {
      题干: 'CPU 中存放操作数地址的是？',
      选项: '["程序计数器（PC）","指令寄存器（IR）","地址寄存器（AR）","数据寄存器（DR）"]',
      答案: 'C. 地址寄存器（AR）',
      分数: 5,
    },
    'single'
  )
  assert.equal(q.stem, 'CPU 中存放操作数地址的是？')
  assert.equal(q.options.length, 4)
  assert.equal(q.options[2], '地址寄存器（AR）')
  assert.equal(q.answer, 'C')
  assert.equal(q.score, 5)
  assert.deepEqual(optionLines(q).length, 4)
  assert.equal(answerLabel(q, 'single'), 'C. 地址寄存器（AR）')
})

test('normalizeGeneratedQuestions：畸形数据经归一化后可正常入库', () => {
  const [q] = normalizeGeneratedQuestions(
    [{ 题目: '1+1=?', 选项: ['1', '2', '3', '4'], 答案: 'B. 2', invalid: false }],
    'single'
  )
  assert.equal(isQuestionUsable(q), true)
  assert.equal(payloadProblem(toAddQuestionPayload(q, 'single')), null)
})

test('normalizeQuestionShape：没选项时保留答案原文，不把信息吞掉', () => {
  const q = normalizeQuestionShape({ stem: '', answer: 'C. 地址寄存器（AR）' }, 'single')
  assert.deepEqual(q.options, [])
  assert.equal(q.answer, 'C. 地址寄存器（AR）')
  assert.equal(q.stem, '')
})
