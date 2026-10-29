package class

import "time"

// 考试阶段（时间轴），与"我交没交卷"是两件事，必须分开表达。
//
// 为什么单独抽成函数：改造前前端只拿到"有没有提交记录"，
// 把空记录一律显示成"未开始"，结果昨天就结束的考试也写着"未开始"。
// 阶段判定放在服务端，用服务器时间，避免客户端时钟不可信。
const (
	PhaseUpcoming = "upcoming" // 还没到开考时间
	PhaseOngoing  = "ongoing"  // 正在进行（含刚好到截止时刻，与交卷判定一致）
	PhaseEnded    = "ended"    // 已结束
)

// ExamPhase 按服务器时间判定考试阶段。
//
// 边界取法：判卷接口用 "now.After(endAt) 才拒绝"，所以在 endAt 这一刻仍然可以交卷，
// 这里也用同样的边界，保证列表显示"进行中"时点进去确实还能交。
func ExamPhase(now, startAt, endAt time.Time) string {
	if now.Before(startAt) {
		return PhaseUpcoming
	}
	if now.After(endAt) {
		return PhaseEnded
	}
	return PhaseOngoing
}
