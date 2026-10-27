package class

import (
	"testing"
	"time"
)

// 阶段判定的边界必须和"能不能交卷"对齐，否则会出现
// 列表写"进行中"、点进去却报"考试已结束"。
func TestExamPhase(t *testing.T) {
	start := time.Date(2026, 10, 26, 16, 32, 0, 0, time.Local)
	end := time.Date(2026, 10, 26, 17, 37, 0, 0, time.Local)

	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"开始前 1 分钟", start.Add(-time.Minute), PhaseUpcoming},
		{"刚好到开始", start, PhaseOngoing},
		{"考试中", start.Add(30 * time.Minute), PhaseOngoing},
		{"刚好到结束", end, PhaseOngoing}, // 与 Submit 的 now.After(endAt) 边界一致
		{"结束后 1 秒", end.Add(time.Second), PhaseEnded},
		{"结束一天后", end.Add(24 * time.Hour), PhaseEnded},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExamPhase(c.now, start, end); got != c.want {
				t.Fatalf("ExamPhase(%s)=%s，期望 %s", c.now.Format("15:04:05"), got, c.want)
			}
		})
	}
}

// 用截图里那批真实考试数据回归：8 场里应有 2 场仍在进行、6 场已结束。
func TestExamPhase_RealDataFromScreenshot(t *testing.T) {
	now := time.Date(2026, 10, 27, 21, 49, 0, 0, time.Local)
	type exam struct {
		title string
		start time.Time
		end   time.Time
		want  string
	}
	exams := []exam{
		{"E2E Exam on class2", time.Date(2026, 10, 26, 16, 32, 0, 0, time.Local), time.Date(2026, 10, 26, 17, 37, 0, 0, time.Local), PhaseEnded},
		{"E2E Test", time.Date(2026, 10, 26, 16, 30, 0, 0, time.Local), time.Date(2026, 10, 26, 17, 35, 0, 0, time.Local), PhaseEnded},
		{"E2E Test Exam", time.Date(2026, 10, 26, 16, 29, 0, 0, time.Local), time.Date(2026, 10, 26, 17, 34, 0, 0, time.Local), PhaseEnded},
		{"期中考试", time.Date(2026, 10, 26, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 27, 0, 0, 0, 0, time.Local), PhaseEnded},
		{"第一次考试", time.Date(2026, 10, 25, 12, 28, 0, 0, time.Local), time.Date(2026, 10, 25, 13, 28, 0, 0, time.Local), PhaseEnded},
		{"第二次考试", time.Date(2026, 10, 19, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 30, 0, 0, 0, 0, time.Local), PhaseOngoing},
		{"其中考试", time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 30, 0, 0, 0, 0, time.Local), PhaseOngoing},
		{"第一次考试(旧)", time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local), time.Date(2026, 10, 13, 0, 0, 0, 0, time.Local), PhaseEnded},
	}
	ongoing, ended := 0, 0
	for _, e := range exams {
		got := ExamPhase(now, e.start, e.end)
		if got != e.want {
			t.Fatalf("%s 应为 %s，实际 %s", e.title, e.want, got)
		}
		if got == PhaseOngoing {
			ongoing++
		} else if got == PhaseEnded {
			ended++
		}
	}
	if ongoing != 2 || ended != 6 {
		t.Fatalf("应 2 场进行中、6 场已结束，实际 %d / %d", ongoing, ended)
	}
}
