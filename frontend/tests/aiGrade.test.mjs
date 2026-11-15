// 主观题 AI 初评的展示/采纳规则单测。
// 红线只有一条：AI 的结果只能进输入框，不能自动变成成绩。
import { test } from 'node:test'
import assert from 'node:assert/strict'

import {
  AI_GRADE_POLL_MS,
  buildComment,
  countEssayQuestions,
  defaultFinalScores,
  displayScore,
  gradeTag,
  manualRows,
  mergeComment,
  objectiveVerdict,
  pollTimedOut,
  shouldPoll,
  totalWithObjective,
  triggerDisabledReason,
} from '../src/utils/aiGrade.js'

test('只有 processing 才轮询，done/manual/failed 都停', () => {
  assert.equal(shouldPoll({ status: 'processing' }), true)
  for (const s of ['done', 'manual', 'failed', '']) {
    assert.equal(shouldPoll({ status: s }), false)
  }
  assert.equal(shouldPoll(null), false)
})

test('轮询间隔 2.5 秒，超过 5 分钟放弃（worker 最多重试 2 次）', () => {
  assert.equal(AI_GRADE_POLL_MS, 2500)
  assert.equal(pollTimedOut(299999), false)
  assert.equal(pollTimedOut(300000), true)
})

test('gradeTag 把五种状态翻成人话', () => {
  assert.deepEqual(gradeTag(null), { text: '未发起 AI 初评', type: 'info' })
  assert.equal(gradeTag({ status: 'processing' }).text, 'AI 批改中')
  assert.equal(gradeTag({ status: 'done' }).text, 'AI 初评已出')
  assert.equal(gradeTag({ status: 'manual' }).text, '建议人工批改')
  assert.equal(gradeTag({ status: 'failed' }).type, 'danger')
})

test('按钮禁用原因：未配置 > 没有主观题 > 可点', () => {
  assert.equal(triggerDisabledReason(false, 3), 'AI 功能未启用')
  assert.equal(triggerDisabledReason(true, 0), '这份提交没有主观题')
  assert.equal(triggerDisabledReason(true, 2), '')
})

test('displayScore 夹到 [0, 满分]，AI 越界不会显示 120 分', () => {
  assert.equal(displayScore({ score: 150, maxScore: 100 }, 100), 100)
  assert.equal(displayScore({ score: -3, maxScore: 100 }, 100), 0)
  assert.equal(displayScore({ score: 18, maxScore: 20 }, 100), 18)
  // 作业满分优先于 AI 自带的 maxScore（老师改过分值时不能按旧的算）
  assert.equal(displayScore({ score: 90, maxScore: 100 }, 50), 50)
  assert.equal(displayScore({ score: 'abc' }, 100), null)
  assert.equal(displayScore(null, 100), null)
})

test('manualRows 只列老师要手批的题，题号按整卷顺序', () => {
  const questions = [
    { id: 11, type: 'single', stem: '单选', score: 5 },
    { id: 12, type: 'essay', stem: '第一道主观题', score: 10 },
    { id: 13, type: 'essay', stem: '第二道主观题', score: 8 },
  ]
  const rows = manualRows(
    [
      { questionId: 12, score: 6, maxScore: 10, comment: '要点齐', errorPoints: ['漏了条件'], suggestion: '补定义' },
      { questionId: 99, score: null, maxScore: 5, comment: '', errorPoints: [], suggestion: '' },
      null,
    ],
    questions
  )
  assert.equal(rows.length, 2)
  assert.equal(rows[0].questionId, 12)
  assert.equal(rows[0].label, '第 2 题')
  assert.equal(rows[0].stem, '第一道主观题')
  assert.equal(rows[0].aiScore, 6)
  assert.equal(rows[0].scoreText, '6 / 10')
  // 题库里的分值优先于 AI 自报的满分（老师改过分值后不能按旧值算）
  assert.equal(rows[1].maxScore, 8)
  assert.equal(rows[1].aiScore, null)
  assert.equal(rows[1].scoreText, '未给分')
})

test('manualRows 考试要把填空/附件一起列为手批题', () => {
  const questions = [
    { id: 1, type: 'single', stem: '选择' },
    { id: 2, type: 'fill', stem: '填空', score: 4 },
    { id: 3, type: 'fileupload', stem: '附件', score: 6 },
    { id: 4, type: 'essay', stem: '简答', score: 10 },
  ]
  assert.deepEqual(manualRows([], questions, ['essay']).map((r) => r.questionId), [4])
  // 考试的填空不自动判分（score.go），不列出来这 4 分就永远加不进总分
  assert.deepEqual(
    manualRows([], questions, ['essay', 'fill', 'fileupload']).map((r) => r.questionId),
    [2, 3, 4]
  )
})

test('manualRows 没有题库时退回 AI 结果，至少还能逐题改分', () => {
  const rows = manualRows([{ questionId: 7, score: 3, maxScore: 5, comment: '一般' }], [])
  assert.equal(rows.length, 1)
  assert.equal(rows[0].label, '题目 #7')
  assert.equal(rows[0].aiScore, 3)
  assert.equal(rows[0].comment, '一般')
})

test('defaultFinalScores：已存逐题分 > AI 初评分 > 0，并夹到题目满分', () => {
  const rows = [
    { questionId: 12, maxScore: 10, aiScore: 6 },
    { questionId: 13, maxScore: 8, aiScore: null },
    { questionId: 14, maxScore: 5, aiScore: 2 },
  ]
  const finals = defaultFinalScores(rows, { 12: 9, 13: 4 })
  assert.deepEqual(finals, { 12: 9, 13: 4, 14: 2 })
  // 越界的历史数据夹回 [0, 满分]
  assert.deepEqual(defaultFinalScores(rows, { 12: 99 }), { 12: 10, 13: 0, 14: 2 })
  assert.deepEqual(defaultFinalScores(rows, { 12: -3 }), { 12: 0, 13: 0, 14: 2 })
})

test('totalWithObjective：总分 = 客观题分 + 各主观题最终分，先累加再取整', () => {
  const rows = [
    { questionId: 12, maxScore: 10 },
    { questionId: 13, maxScore: 8 },
  ]
  assert.deepEqual(totalWithObjective(5, rows, { 12: 6, 13: 8 }), {
    objective: 5,
    subjective: 14,
    total: 19,
  })
  // 客观题 0 分也要算进去（纯主观作业不能因为 0 就被当成"没有客观题"）
  assert.equal(totalWithObjective(0, rows, { 12: 3 }).total, 3)
  // 逐题分越界按题目满分封顶，负数按 0
  assert.equal(totalWithObjective(2, rows, { 12: 999, 13: -1 }).total, 12)
  // 多选按权重会产生 .5 分：合计先保留一位小数，总分取整（后端 score 是 int）
  assert.equal(totalWithObjective(7.5, [{ questionId: 1, maxScore: 5 }], { 1: 2.5 }).subjective, 2.5)
  assert.equal(totalWithObjective(7.5, [{ questionId: 1, maxScore: 5 }], { 1: 2.5 }).total, 10)
  // 题目分值加起来超过卷面总分时仍然夹到满分（后端会拒绝越界分数，前端先夹住）
  assert.equal(totalWithObjective(95, rows, { 12: 10, 13: 8 }, 100).total, 100)
  assert.equal(totalWithObjective(10, rows, { 12: 5 }, 100).total, 15)
  assert.deepEqual(totalWithObjective(null, [], {}), { objective: 0, subjective: 0, total: 0 })
})

test('buildComment 用最终分而不是 AI 分，评语和保存的分数对得上', () => {
  const grade = {
    status: 'done',
    score: 16,
    maxScore: 20,
    comment: 'AI 初评：2 道主观题',
    items: [
      { questionId: 12, score: 10, maxScore: 10, comment: '完整', errorPoints: [], suggestion: '' },
      { questionId: 13, score: 6, maxScore: 10, comment: '漏要点', errorPoints: ['没写备择假设'], suggestion: '再看第三章' },
    ],
  }
  const rows = manualRows(grade.items, [
    { id: 12, type: 'essay', stem: '题一', score: 10 },
    { id: 13, type: 'essay', stem: '题二', score: 10 },
  ])
  // 老师把第 1 题从 10 改成 8
  const comment = buildComment(grade, rows, { 12: 8, 13: 6 })
  assert.match(comment, /^AI 初评：2 道主观题\n/)
  assert.match(comment, /第 1 题 8 \/ 10：完整/)
  assert.match(comment, /第 2 题 6 \/ 10：漏要点/)
  assert.match(comment, /· 问题：没写备择假设/)
  assert.match(comment, /· 建议：再看第三章/)
})

test('mergeComment 反复采纳时原位替换上次 AI 段落，老师自己写的话不动', () => {
  const first = '第 1 题 8 / 10：完整'
  assert.equal(mergeComment('', first, ''), first)
  // 老师没动过评语：第二次采纳直接替换，不会堆两遍
  assert.equal(mergeComment(first, '第 1 题 9 / 10：完整', first), '第 1 题 9 / 10：完整')
  // 老师在前面补了一句话：AI 段落原位替换，人工内容保留
  assert.equal(
    mergeComment('整体不错。\n\n' + first, '第 1 题 9 / 10：完整', first),
    '整体不错。\n\n第 1 题 9 / 10：完整'
  )
  // 找不到上次段落（老师手工删了）：按新内容追加，不丢人工评语
  assert.equal(mergeComment('自己写的', first, '不存在的旧段落'), '自己写的\n\n' + first)
})

test('countEssayQuestions：AI 侧只批 essay', () => {
  assert.equal(
    countEssayQuestions([
      { type: 'single' },
      { type: 'essay' },
      { type: 'essay' },
      { type: 'fileupload' },
    ]),
    2
  )
  assert.equal(countEssayQuestions(undefined), 0)
})

// ===== 三处答卷页共用的对错判定 =====

test('objectiveVerdict 三处答卷页统一的对错判定', () => {
  // 单选
  assert.equal(objectiveVerdict('A', 'A', 'single'), 'correct')
  assert.equal(objectiveVerdict('B', 'A', 'single'), 'wrong')
  // 多选：按集合比较，顺序无关
  assert.equal(objectiveVerdict('A,C', 'C,A', 'multiple'), 'correct')
  assert.equal(objectiveVerdict('A,B', 'A,C', 'multiple'), 'wrong')
  // 判断题归一化：历史数据里对/错存成过 true/false、对/错、A/B
  assert.equal(objectiveVerdict('true', '对', 'judge'), 'correct')
  assert.equal(objectiveVerdict('A', 'true', 'judge'), 'correct')
  assert.equal(objectiveVerdict('错', 'B', 'judge'), 'correct')
  assert.equal(objectiveVerdict('false', 'true', 'judge'), 'wrong')
  // 主观题 / 无答案：不给结论，不误导
  assert.equal(objectiveVerdict('任意作答', '', 'essay'), '')
  assert.equal(objectiveVerdict('', 'A', 'single'), '')
  assert.equal(objectiveVerdict('随便', '随便', 'fileupload'), '')
  // 填空：与后端自动判分同口径（忽略大小写）
  assert.equal(objectiveVerdict('光电效应', '光电效应', 'fill'), 'correct')
  assert.equal(objectiveVerdict('Photo', 'photo', 'fill'), 'correct')
  assert.equal(objectiveVerdict('错答', '标准答案', 'fill'), 'wrong')
})
