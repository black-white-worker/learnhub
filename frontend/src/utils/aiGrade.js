// 主观题 AI 初评的展示与轮询规则（纯逻辑，便于脱离浏览器单测）。
//
// 后端约定（见 backend/internal/app/class/logic/aigradelogic.go）：
//   status 空 = 没发起过；processing = 排队/批改中；done = 有结果；
//   manual = AI 批不动，请人工；failed = 失败（error 里有原因）。
// 关键红线：AI 只写建议表，成绩册只能由老师落笔，所以这里所有"采纳"动作
// 都只是往输入框里填值，绝不自动保存分数。

export const AI_GRADE_POLL_MS = 2500
// 一次批改最长等 5 分钟：worker 的额度是「LLM 单次 60 秒 × (1 + 重试 2 次)」，
// 再往上就是 worker 真挂了，继续轮询只是白打接口。
export const AI_GRADE_POLL_MAX_MS = 300000

export function shouldPoll(grade) {
  return grade?.status === 'processing'
}

export function pollTimedOut(elapsedMs) {
  return elapsedMs >= AI_GRADE_POLL_MAX_MS
}

// 状态标签文案 + 颜色（Element Plus 的 tag type）。
export function gradeTag(grade) {
  const status = grade?.status || ''
  if (status === 'processing') return { text: 'AI 批改中', type: 'warning' }
  if (status === 'done') return { text: 'AI 初评已出', type: 'success' }
  if (status === 'manual') return { text: '建议人工批改', type: 'warning' }
  if (status === 'failed') return { text: 'AI 批改失败', type: 'danger' }
  return { text: '未发起 AI 初评', type: 'info' }
}

// 按钮不可用时的原因；返回空串表示可以点。
export function triggerDisabledReason(aiEnabled, essayCount) {
  if (!aiEnabled) return 'AI 功能未启用'
  if ((Number(essayCount) || 0) === 0) return '这份提交没有主观题'
  return ''
}

// 有结果时把总分夹到 [0, maxScore]：AI 越界的数据不该让老师看到 120 分。
export function displayScore(grade, maxScore) {
  const raw = Number(grade?.score)
  if (!Number.isFinite(raw)) return null
  const upper = Number(maxScore) > 0 ? Number(maxScore) : Number(grade?.maxScore) || 100
  return Math.min(Math.max(Math.round(raw), 0), Math.max(upper, 0))
}

// clampScore 把单题分数夹到 [0, 该题满分]。满分缺失（老数据没带 maxScore）时不设上限，
// 但仍然不许为负 —— 负数落库会让「逐题分加起来」小于客观分。
export function clampScore(value, maxScore) {
  // 空值必须原样返回 null：Number(null) === 0，直接转数字会把"AI 没给分"
  // 变成"AI 给了 0 分"，逐题输入框也会被默认填成 0。
  if (value === null || value === undefined || value === '') return null
  const n = Number(value)
  if (!Number.isFinite(n)) return null
  const lower = Math.max(n, 0)
  const upper = Number(maxScore) > 0 ? Number(maxScore) : null
  return upper === null ? lower : Math.min(lower, upper)
}

// manualRows 把「需要老师手批的题」列成逐题评分行。
//
// 为什么要按题库来列、而不是按 AI 返回的 items 列：
//   AI 只批 essay，考试里的填空/附件题它根本不看；如果只列 AI 返回的题，
//   这些题的分就没地方落，累加出来的总分必然偏低。
// types 决定哪些题型算「手批」：作业默认只 essay（填空在作业里是自动判分），
// 考试传 essay/fill/fileupload（考试的填空不自动判，见 exam/logic/score.go）。
export function manualRows(items, questions, types = ['essay']) {
  const allowed = new Set((types || []).map(String))
  const itemByQid = new Map()
  for (const it of items || []) {
    if (!it || typeof it !== 'object') continue
    const qid = Number(it.questionId)
    if (Number.isFinite(qid)) itemByQid.set(qid, it)
  }

  const rows = []
  let hasQuestionList = false
  ;(questions || []).forEach((q, i) => {
    const qid = Number(q?.id ?? q?.Id)
    if (!Number.isFinite(qid)) return
    hasQuestionList = true
    const type = String(q?.type || q?.Type || '')
    if (!allowed.has(type)) return
    rows.push(rowOf(qid, i + 1, q, itemByQid.get(qid)))
  })

  // 调用方没传题库（或题库一条都没加载到）时退回 AI 结果：
  // 至少还能逐题改分，不至于整块面板消失。
  if (!hasQuestionList) {
    for (const [qid, it] of itemByQid) rows.push(rowOf(qid, 0, null, it))
  }
  return rows
}

function rowOf(qid, seq, question, item) {
  const questionMax = Number(question?.score)
  const itemMax = Number(item?.maxScore)
  const maxScore = questionMax > 0 ? questionMax : itemMax > 0 ? itemMax : null
  const raw = item && item.score !== null && item.score !== undefined ? Number(item.score) : null
  const aiScore = clampScore(raw, maxScore)
  return {
    questionId: qid,
    label: seq > 0 ? `第 ${seq} 题` : `题目 #${qid}`,
    stem: String(question?.stem || question?.title || ''),
    maxScore,
    aiScore,
    hasAi: Boolean(item),
    scoreText: aiScore === null ? '未给分' : `${aiScore} / ${maxScore ?? '?'}`,
    comment: String(item?.comment || '').trim(),
    errorPoints: Array.isArray(item?.errorPoints) ? item.errorPoints.filter(Boolean).map(String) : [],
    suggestion: String(item?.suggestion || '').trim(),
    parseOk: item?.parseOk !== false,
  }
}

// defaultFinalScores 每道手批题的「最终评分」初始值：
//   已保存的逐题分 > AI 初评分 > 0。
// 逐题分优先是为了不覆盖老师上次的批改；AI 分只是默认填充，随时可改。
export function defaultFinalScores(rows, storedScores) {
  const out = {}
  for (const row of rows || []) {
    const raw = storedScores?.[row.questionId] ?? storedScores?.[String(row.questionId)]
    const saved = raw === null || raw === undefined || raw === '' ? null : clampScore(raw, row.maxScore)
    out[row.questionId] = saved === null ? row.aiScore ?? 0 : saved
  }
  return out
}

// totalWithObjective 最终总分的唯一算法：客观题自动分 + 主观题逐题最终分之和。
//
// 总分取整是因为 exam_submissions.teacher_score / assignment_submissions.score
// 都是 int；逐题分允许一位小数（多选按权重给分会产生 .5），所以先累加再取整。
// 返回的三个数都用于界面展示，老师能一眼看到「这个总分是怎么来的」。
export function totalWithObjective(objectiveScore, rows, finalScores, maxScore) {
  const objective = Math.max(Number(objectiveScore) || 0, 0)
  let subjective = 0
  for (const row of rows || []) {
    const v = clampScore(finalScores?.[row.questionId], row.maxScore)
    if (v !== null) subjective += v
  }
  const round1 = (n) => Math.round(n * 10) / 10
  const total = Math.max(Math.round(objective + subjective), 0)
  // 题目分值配置加起来超过卷面总分时（历史数据常见），仍然不许突破满分：
  // 后端批改接口本身会拒绝越界分数，前端先夹住，免得老师点了保存才报错。
  const upper = Number(maxScore) > 0 ? Number(maxScore) : null
  return {
    objective: round1(objective),
    subjective: round1(subjective),
    total: upper === null ? total : Math.min(total, upper),
  }
}

// buildComment 采纳时写进评语框的内容：AI 总评 + 逐题「最终分 / 满分 + AI 评语」。
// 用最终分而不是 AI 分，评语才和保存的分数对得上。
export function buildComment(grade, rows, finalScores) {
  const lines = []
  const comment = String(grade?.comment || '').trim()
  if (comment) lines.push(comment)
  for (const row of rows || []) {
    const v = clampScore(finalScores?.[row.questionId], row.maxScore)
    if (v === null && !row.comment && !row.errorPoints.length) continue
    const scoreText = v === null ? '未给分' : `${v} / ${row.maxScore ?? '?'}`
    lines.push(`${row.label} ${scoreText}${row.comment ? '：' + row.comment : ''}`)
    for (const p of row.errorPoints) lines.push(`  · 问题：${p}`)
    if (row.suggestion) lines.push(`  · 建议：${row.suggestion}`)
  }
  return lines.join('\n')
}

// mergeComment 反复采纳时，替换上一次由 AI 写入的那一段而不是无限追加。
//
// 新流程下老师会「改一题 → 再采纳一次」，直接 append 会把同一份逐题评语
// 在评语框里堆两遍。previous 是上次写入的原文：找到就原位替换，
// 老师自己补的话（前后文）保持不动。
export function mergeComment(current, incoming, previous) {
  const text = String(incoming || '').trim()
  if (!text) return String(current || '')
  const cur = String(current || '')
  if (!cur) return text
  if (previous && cur.includes(previous)) return cur.replace(previous, text)
  return cur + '\n\n' + text
}

// objectiveVerdict 客观题在「查看答卷 / 逐份批阅」里统一的对错判定。
// 返回 'correct' / 'wrong' / ''（主观题或无答案记录时不给结论，不误导）。
// 覆盖范围与后端自动判分一致：单选/多选/判断 + 作业的填空（精确比对，自动判分）。
// 判断题做归一化：历史数据里对/错存成过 true/false、对/错、A/B 三种写法。
export function objectiveVerdict(studentAnswer, standardAnswer, type) {
  const t = String(type || '')
  if (!['single', 'multiple', 'judge', 'fill'].includes(t)) return ''
  const norm = (v) => {
    const s = String(v ?? '').trim()
    if (t === 'judge') {
      const u = s.toUpperCase()
      if (['A', 'TRUE', 'T', '1', '对', '√', '正确'].includes(u)) return 'TRUE'
      if (['B', 'FALSE', 'F', '0', '错', '×', 'X', '错误'].includes(u)) return 'FALSE'
    }
    return s
  }
  const student = norm(studentAnswer)
  const standard = norm(standardAnswer)
  if (!student || !standard) return ''
  if (t === 'multiple') {
    const set = (s) => [...new Set(s.split(',').map((x) => x.trim()).filter(Boolean))].sort().join(',')
    return set(student) === set(standard) ? 'correct' : 'wrong'
  }
  if (t === 'fill') {
    // 与后端 strings.EqualFold 对齐：填空比对忽略大小写
    return student.toLowerCase() === standard.toLowerCase() ? 'correct' : 'wrong'
  }
  return student === standard ? 'correct' : 'wrong'
}

// 主观题数量：后端已经给了 essayCount，但这个字段可能来自旧数据，前端自己数一遍更稳。
// 注意这里只数 essay：AI 批量初评覆盖的题型就是 essay，
// 考试的填空/附件题虽然也要手批，但没有 AI 分可采纳。
export function countEssayQuestions(questions) {
  return (questions || []).filter((q) => (q?.type || q?.Type) === 'essay').length
}
