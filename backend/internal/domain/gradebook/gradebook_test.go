package gradebook

import (
	"math"
	"testing"
)

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 0.05 }

func TestPercent(t *testing.T) {
	cases := []struct{ score, max, want float64 }{
		{50, 100, 50},
		{0, 100, 0},
		{100, 100, 100},
		{45, 50, 90},
		{90, 50, 100}, // 超过满分要封顶
		{10, 0, 0},    // 分母为 0 不能除零
		{-5, 100, 0},  // 负分按 0
	}
	for _, c := range cases {
		if got := Percent(c.score, c.max); !almostEqual(got, c.want) {
			t.Fatalf("Percent(%v,%v)=%v，期望 %v", c.score, c.max, got, c.want)
		}
	}
}

func TestRatio(t *testing.T) {
	if got := Ratio(3, 4); !almostEqual(got, 75) {
		t.Fatalf("3/4 应为 75，实际 %v", got)
	}
	if got := Ratio(0, 0); got != 0 {
		t.Fatalf("总数为 0 应返回 0，实际 %v", got)
	}
	if got := Ratio(5, 4); !almostEqual(got, 100) {
		t.Fatalf("完成数超过总数应封顶 100，实际 %v", got)
	}
}

// 旧实现的问题：视频项被写死成满分（100*weight/100），
// 这里验证"一个视频都没看的学生，视频类必须是 0 分"。
func TestTotal_VideoZeroWhenNothingWatched(t *testing.T) {
	p := map[string]float64{"video": 0, "assignment": 80, "signin": 100, "exam": 90}
	got := Total(p, DefaultWeights())
	// 0*30 + 80*50 + 100*10 + 90*10 = 4000+1000+900 = 5900 / 100 = 59
	if !almostEqual(got, 59) {
		t.Fatalf("期望 59，实际 %v", got)
	}
}

func TestTotal_SigninCannotExceedWeight(t *testing.T) {
	// 签到 30 次（Ratio 会封顶 100），不应把总分顶穿
	p := map[string]float64{"video": 100, "assignment": 100, "signin": 100, "exam": 100}
	got := Total(p, DefaultWeights())
	if !almostEqual(got, 100) {
		t.Fatalf("全满应得 100，实际 %v", got)
	}
}

func TestTotal_WeightChange(t *testing.T) {
	p := map[string]float64{"video": 0, "assignment": 100, "signin": 0, "exam": 0}
	w := Weights{Video: 0, Assignment: 100, Signin: 0, Exam: 0}
	if got := Total(p, w); !almostEqual(got, 100) {
		t.Fatalf("作业权重 100 且满分应得 100，实际 %v", got)
	}
}

// 全 0 权重时退回默认权重，避免出现"总分 0"这种假数据。
func TestTotal_ZeroWeightsFallback(t *testing.T) {
	p := map[string]float64{"video": 100, "assignment": 100, "signin": 100, "exam": 100}
	if got := Total(p, Weights{}); !almostEqual(got, 100) {
		t.Fatalf("全 0 权重应退回默认权重，实际 %v", got)
	}
}

// 空分类不占权重：班里还没布置过考试时，考试权重不应拉低总分。
func TestEffectiveWeights_DropsEmptyCategories(t *testing.T) {
	w := EffectiveWeights(DefaultWeights(), true, true, false, false)
	if w.Signin != 0 || w.Exam != 0 {
		t.Fatalf("空分类权重应置零，实际 %+v", w)
	}
	if w.Video != 30 || w.Assignment != 50 {
		t.Fatalf("非空分类权重不应变化，实际 %+v", w)
	}
	p := map[string]float64{"video": 100, "assignment": 100}
	if got := Total(p, w); !almostEqual(got, 100) {
		t.Fatalf("只有视频+作业且都满分应为 100，实际 %v", got)
	}
}

func TestLetter(t *testing.T) {
	cases := map[float64]string{95: "A", 90: "A", 85: "B", 75: "C", 60: "D", 59.9: "F", 0: "F"}
	for p, want := range cases {
		if got := Letter(p); got != want {
			t.Fatalf("Letter(%v)=%s，期望 %s", p, got, want)
		}
	}
}
