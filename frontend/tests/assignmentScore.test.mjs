// 作业最终成绩的计算与展示单测。
//
// 回归重点：修复「主观题评分显示 222.0 分」缺陷。
// 根因：未批改时把客观题自动分算了两次
//   (autoScore) + (score ?? autoScore) = 111 + 111 = 222.0
// 这些用例锁定「展示只用 score，score 缺失才退回 autoScore（且绝不相加）」的口径，
// 防止双击计数（double-count）再次混入。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  computeFinalAssignmentScore,
  displayAssignmentScore,
  sumEssayScores,
} from '../src/utils/assignmentScore.js'

// ---------- sumEssayScores ----------

test('sumEssayScores：对象按题号汇总，空值当 0', () => {
  assert.equal(sumEssayScores({ 1: 10, 2: 5, 3: null }), 15)
  assert.equal(sumEssayScores({ 0: 25 }), 25)
})

test('sumEssayScores：数组形式也能汇总', () => {
  assert.equal(sumEssayScores([10, 5, 0]), 15)
  assert.equal(sumEssayScores([]), 0)
})

test('sumEssayScores：null / undefined 返回 0，不报错', () => {
  assert.equal(sumEssayScores(null), 0)
  assert.equal(sumEssayScores(undefined), 0)
})

// ---------- computeFinalAssignmentScore（保存批改时下发的总分）----------

test('computeFinalAssignmentScore：客观自动分 + 主观手批分合计', () => {
  // 对象写法（AssignmentTab 实际用法）
  assert.equal(computeFinalAssignmentScore(60, { 1: 20, 2: 5 }), 85)
  // 单道主观题包装成 { 0: essayTotal }
  assert.equal(computeFinalAssignmentScore(60, { 0: 25 }), 85)
  // 数组写法
  assert.equal(computeFinalAssignmentScore(60, [25]), 85)
})

test('computeFinalAssignmentScore：没有主观题时就是客观自动分', () => {
  assert.equal(computeFinalAssignmentScore(111, {}), 111)
  assert.equal(computeFinalAssignmentScore(111, null), 111)
})

test('computeFinalAssignmentScore：脏数据（NaN/字符串）不污染结果', () => {
  assert.equal(computeFinalAssignmentScore('abc', { 1: 'x' }), 0)
  assert.equal(computeFinalAssignmentScore(50, { 1: '20' }), 70)
})

// ---------- displayAssignmentScore（列表/弹窗展示的总分）----------

test('displayAssignmentScore：已批改以 score 为准（客观+主观的合计）', () => {
  assert.equal(displayAssignmentScore({ autoScore: 60, score: 85 }), 85)
  assert.equal(displayAssignmentScore({ autoScore: 0, score: 85 }), 85)
})

test('displayAssignmentScore：未批改退回客观题自动分', () => {
  assert.equal(displayAssignmentScore({ autoScore: 111 }), 111)
  assert.equal(displayAssignmentScore({ autoScore: 111, score: null }), 111)
  assert.equal(displayAssignmentScore({ autoScore: 111, score: undefined }), 111)
})

// 核心回归：曾经 (autoScore) + (score ?? autoScore) = 111 + 111 = 222.0
test('回归：未批改的答卷绝不能把客观分算两遍（曾经显示 222.0）', () => {
  const sub = { autoScore: 111 }
  assert.equal(displayAssignmentScore(sub), 111)
  // 即便 shape 和修复前一模一样（有 autoScore、无 score），也只显示一次
  assert.notEqual(displayAssignmentScore(sub), 222)
})

test('displayAssignmentScore：score 为 0 也要正确显示 0，而不是退回 autoScore', () => {
  assert.equal(displayAssignmentScore({ autoScore: 111, score: 0 }), 0)
})

test('displayAssignmentScore：什么都没有返回 0', () => {
  assert.equal(displayAssignmentScore({}), 0)
  assert.equal(displayAssignmentScore(null), 0)
  assert.equal(displayAssignmentScore(undefined), 0)
  assert.equal(displayAssignmentScore(), 0)
})

// ---------- 端到端契约一致性 ----------

// 模拟「老师逐题打完主观题 → 计算最终分 → 展示」整条链路，
// 证明保存下发的 score 与展示用的分数口径一致，不会出现双倍。
test('端到端：主观题手批 25 分 + 客观 60 分 → 保存与展示都是 85', () => {
  const autoScore = 60
  const essayScores = { 0: 25 } // AssignmentTab.saveGrade 里的写法
  const finalScore = computeFinalAssignmentScore(autoScore, essayScores)

  // 保存后后端把 finalScore 写进 score 字段
  const savedSubmission = { autoScore, score: finalScore }
  // 展示侧和保存侧必须一致
  assert.equal(displayAssignmentScore(savedSubmission), finalScore)
  assert.equal(finalScore, 85)
})
