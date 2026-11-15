// 作业最终成绩的计算与展示（纯函数，便于单测）。
//
// 口径（与后端 /assignment/submission/:sid/grade 对齐）：
//   后端把 req.Score 当作「这份作业的最终成绩」直接写进成绩册，
//   所以前端下发的 score 必须是「客观题自动分 + 主观题手批分」的合计，
//   不能只传主观题部分（否则客观题分会被丢掉，详见 AssignmentTab.saveGrade 的修复）。

// sumEssayScores 把主观题逐题手批分汇总。
// essayScores 支持 {questionId: 分数} 对象或 [分数...] 数组，空值按 0 处理。
export function sumEssayScores(essayScores) {
  if (!essayScores) return 0
  let total = 0
  if (Array.isArray(essayScores)) {
    for (const v of essayScores) total += Number(v) || 0
    return total
  }
  for (const k of Object.keys(essayScores)) total += Number(essayScores[k]) || 0
  return total
}

// computeFinalAssignmentScore 最终成绩 = 客观题自动分 + 主观题手批分。
// 这是「保存批改」时应当下发给后端的总分。
export function computeFinalAssignmentScore(autoScore, essayScores) {
  return (Number(autoScore) || 0) + sumEssayScores(essayScores)
}

// displayAssignmentScore 展示用最终成绩：
//   - 已批改以 score 为准；
//   - 未批改退回客观题自动分；
//   - 都没有则 0。
// 关键：绝不能把 autoScore 和 score 相加——score 已经是「客观+主观」的合计，
// 否则会出现「同一份客观分被算两遍」导致分数翻倍（如 222.0 分）。
export function displayAssignmentScore(submission) {
  if (!submission) return 0
  const score = submission.score
  if (score !== null && score !== undefined) return Number(score) || 0
  const auto = submission.autoScore
  if (auto !== null && auto !== undefined) return Number(auto) || 0
  return 0
}
