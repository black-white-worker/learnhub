// Package gradebook 成绩册的纯计算逻辑。
//
// 抽成独立包的原因和 ScoreExam 一样：权重、封顶、展示分这些规则最容易算错，
// 放在纯函数里可以穷举单测，不依赖数据库；logic 层只负责取数 + 调这里。
//
// 计算模型（对齐 Canvas 的 weighted assignment groups）：
//
//	某类得分率 = 该类 Σ得分 / 该类 Σ满分 × 100
//	学期总分   = Σ(各类得分率 × 该类权重) / Σ权重
//
// 和旧实现的区别：旧代码把"视频"这一项写死成满分 100、签到次数×10 不封顶，
// 导致总分没有教学含义；这里每一项都来自真实数据，且各自封顶 100。
package gradebook

import "math"

// Weights 四类成绩的权重（来自 class_grade_rules，未配置时用默认值）。
type Weights struct {
	Video      int
	Assignment int
	Signin     int
	Exam       int
}

// DefaultWeights 默认权重：视频 30 / 作业 50 / 签到 10 / 考试 10。
func DefaultWeights() Weights {
	return Weights{Video: 30, Assignment: 50, Signin: 10, Exam: 10}
}

// Normalize 把权重规整成非负整数；全为 0 时退回默认权重。
func (w Weights) Normalize() Weights {
	if w.Video < 0 {
		w.Video = 0
	}
	if w.Assignment < 0 {
		w.Assignment = 0
	}
	if w.Signin < 0 {
		w.Signin = 0
	}
	if w.Exam < 0 {
		w.Exam = 0
	}
	if w.Video+w.Assignment+w.Signin+w.Exam == 0 {
		return DefaultWeights()
	}
	return w
}

// Sum 权重之和。
func (w Weights) Sum() int {
	w = w.Normalize()
	return w.Video + w.Assignment + w.Signin + w.Exam
}

// Percent 得分率（0-100）。满分为 0（"还没有这类作业"）时返回 0，
// 由调用方用 HasData 决定是"0 分"还是"未参与"。
func Percent(score, max float64) float64 {
	if max <= 0 {
		return 0
	}
	p := score / max * 100
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// Ratio 比例型得分率：完成数 / 总数 × 100（签到、视频用）。
func Ratio(done, total int) float64 {
	if total <= 0 {
		return 0
	}
	return Percent(float64(done), float64(total))
}

// Total 加权总分（0-100，保留 1 位小数）。
//
// percentages 的 key 固定为 "video" / "assignment" / "signin" / "exam"。
// 缺失的分类按 0 分计入——即"有这项考核但没做"，符合教学口径；
// 若某类根本没有配置（分母为 0），调用方应把该项权重视为 0 后再传进来，
// 否则会出现"没布置作业的学生因为作业项 0 分被拉低"的不公平。
func Total(percentages map[string]float64, w Weights) float64 {
	w = w.Normalize()
	sum := float64(w.Sum())
	if sum <= 0 {
		return 0
	}
	acc := 0.0
	acc += clamp(percentages["video"]) * float64(w.Video)
	acc += clamp(percentages["assignment"]) * float64(w.Assignment)
	acc += clamp(percentages["signin"]) * float64(w.Signin)
	acc += clamp(percentages["exam"]) * float64(w.Exam)
	return round1(acc / sum)
}

// EffectiveWeights 把"没有任何成绩项"的分类权重置零并重新归一，
// 避免空分类白占权重导致全班总分偏低。
func EffectiveWeights(w Weights, hasVideo, hasAssignment, hasSignin, hasExam bool) Weights {
	w = w.Normalize()
	if !hasVideo {
		w.Video = 0
	}
	if !hasAssignment {
		w.Assignment = 0
	}
	if !hasSignin {
		w.Signin = 0
	}
	if !hasExam {
		w.Exam = 0
	}
	if w.Video+w.Assignment+w.Signin+w.Exam == 0 {
		return DefaultWeights()
	}
	return w
}

// Letter 百分制转等级（展示用，非精确换算）。
func Letter(p float64) string {
	switch {
	case p >= 90:
		return "A"
	case p >= 80:
		return "B"
	case p >= 70:
		return "C"
	case p >= 60:
		return "D"
	default:
		return "F"
	}
}

func clamp(v float64) float64 {
	if math.IsNaN(v) || v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
