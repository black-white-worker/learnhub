package class

import (
	"testing"

	"learningplatform/internal/model"
)

func TestSameMultiChoice(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"完全相同", "A,C", "A,C", true},
		{"顺序不同", "A,C", "C,A", true},
		{"大小写不同", "a,c", "A,C", true},
		{"多余空格", "A, C", "A,C", true},
		{"少选", "A,B,C", "A,B", false},
		{"多选", "A,B", "A,B,C", false},
		{"不同选项", "A,C", "A,D", false},
		{"单选项", "B", "B", true},
		{"单选项不同", "B", "C", false},
		{"空值", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SameMultiChoice(tt.a, tt.b); got != tt.want {
				t.Errorf("SameMultiChoice(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGradeAutoAnswersMultiple(t *testing.T) {
	questions := []*model.QuestionRow{
		{Id: 1, Type: "multiple", Answer: "A,C", Score: 10},
		{Id: 2, Type: "single", Answer: "B", Score: 5},
		{Id: 3, Type: "judge", Answer: "true", Score: 5},
		{Id: 4, Type: "essay", Answer: "", Score: 20},
	}
	// 学生：多选选对AC，单选选B，判断对，主观题不参与自动判分
	answers := map[string]string{
		"1": "C,A", // 顺序不同但选项对
		"2": "B",
		"3": "true",
	}
	score := GradeAutoAnswers(questions, answers)
	if score != 20 { // 10+5+5
		t.Errorf("期望得分20，实际 %d", score)
	}
}
