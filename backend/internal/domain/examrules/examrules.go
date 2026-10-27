// Package examrules 考试的"时间规则"与"答案可见性"纯函数。
//
// 抽出来的原因：改造前这些判断散在 ExamLogic 里且只做了一半——
//   - 交卷只校验 start_at/end_at，不看"开始考试 + 时长"；
//   - allow_late / tab_limit 有字段没有逻辑；
//   - 前端倒计时自己按 end_at 算，服务端不掌握时间主权。
//
// 这里把规则固化成一个纯函数集合：logic 只负责取数据 + 调判定，
// 前端拿服务端返回的 deadlineAt 展示，任何人改本地时间都无效。
package examrules

import (
	"errors"
	"time"
)

// 交卷窗口判定的三类失败原因，logic 层据此转成 400 + 可读文案。
var (
	// ErrNotStarted 尚未到开考时间
	ErrNotStarted = errors.New("考试还未开始")
	// ErrEnded 已过考试结束时间（且不允许补交）
	ErrEnded = errors.New("考试已结束")
	// ErrExpired 已超过"个人作答时限"（开考时间 + 时长）
	ErrExpired = errors.New("已超过本次考试时限，无法再提交")
	// ErrAlreadySubmitted 已经交过卷，禁止二次提交
	ErrAlreadySubmitted = errors.New("你已经交卷了，不能重复提交")
)

// Window 一场考试的时间窗口。
type Window struct {
	StartAt      time.Time // 允许开考时间
	EndAt        time.Time // 考试结束时间（所有人）
	DurationMin  int       // 个人作答时长（分钟），<=0 表示只受 EndAt 限制
	AllowLate    bool      // 是否允许在截止后补交
	LateDeadline time.Time // 补交截止时间，零值表示没有单独设置
}

// effectiveDuration 个人作答时长的兜底：没配时长就按整个考试窗口算。
func (w Window) effectiveDuration() time.Duration {
	if w.DurationMin <= 0 {
		return 0
	}
	return time.Duration(w.DurationMin) * time.Minute
}

// Deadline 计算某个学生的个人交卷截止时间 = min(考试结束, 开考时间 + 时长)。
//
// 注意用 startedAt 而不是 StartAt：改造前用 e.StartAt + duration，
// 迟到 20 分钟进考场的人会被凭空扣掉 20 分钟。
func (w Window) Deadline(startedAt time.Time) time.Time {
	deadline := w.EndAt
	if d := w.effectiveDuration(); d > 0 {
		personal := startedAt.Add(d)
		if personal.Before(deadline) {
			deadline = personal
		}
	}
	return deadline
}

// CheckStart 判定能否开考。允许 late 的意义是"可以晚进场"，但进场后
// 仍然受个人时长约束（Deadline 用 startedAt 计算，天然满足）。
func (w Window) CheckStart(now time.Time) error {
	if now.Before(w.StartAt) {
		return ErrNotStarted
	}
	if now.After(w.EndAt) {
		// 结束时间之后不允许新开考；允许补交也不能"开新卷"
		return ErrEnded
	}
	return nil
}

// CheckSubmit 判定能否交卷。
//
// 规则优先级：
//  1. 超过个人截止时间 → 看 allow_late / late_deadline；
//  2. 补交窗口内允许提交（由调用方标记为 late，成绩单照常记录）；
//  3. 否则拒绝，并区分"整场结束"与"个人超时"，方便前端提示。
func (w Window) CheckSubmit(now, startedAt time.Time) error {
	if startedAt.IsZero() {
		// 没有开考记录（例如直接调交卷接口）：只受整场时间约束
		if now.Before(w.StartAt) {
			return ErrNotStarted
		}
		if now.After(w.EndAt) && !w.canLate(now) {
			return ErrEnded
		}
		return nil
	}

	deadline := w.Deadline(startedAt)
	if !now.After(deadline) {
		return nil
	}
	if w.canLate(now) {
		return nil
	}
	// 个人时限已过：如果整场也结束了，报"考试已结束"更符合直觉
	if now.After(w.EndAt) {
		return ErrEnded
	}
	return ErrExpired
}

// canLate 是否处于补交窗口内。
func (w Window) canLate(now time.Time) bool {
	if !w.AllowLate {
		return false
	}
	if w.LateDeadline.IsZero() {
		// 只开了 allow_late 却没给补交截止时间：给 24 小时兜底，
		// 否则这个开关等于"永久可交"，会成为作弊入口。
		return now.Before(w.EndAt.Add(24 * time.Hour))
	}
	return !now.After(w.LateDeadline)
}

// RevealAnswers 交卷后能否把标准答案下发给学生。
//
// 只在整场考试结束后才放开：考试期间先交卷的学生如果立刻拿到答案，
// 可以把答案传给同班还在考的人。老师/管理员不受此限制（调用方另行放行）。
func (w Window) RevealAnswers(now time.Time) bool {
	return now.After(w.EndAt)
}

// AutoSubmitDue 超时自动交卷的扫描条件：开考了、还没交、已经过了个人截止。
func (w Window) AutoSubmitDue(now, startedAt time.Time) bool {
	if startedAt.IsZero() {
		return false
	}
	return now.After(w.Deadline(startedAt))
}
