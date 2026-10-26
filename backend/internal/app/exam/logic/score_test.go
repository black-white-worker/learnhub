package logic

import (
	"testing"

	"learningplatform/internal/model"
)

// 构造一道题的工具函数
func q(id int64, qtype, answer string, score int) *model.ExamQuestionRow {
	return &model.ExamQuestionRow{Id: id, Type: qtype, Answer: answer, Score: score}
}

func TestScoreExam_SingleChoice(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		q(1, "single", "A", 5),
		q(2, "single", "B", 5),
	}
	// 第 1 题答对 A，第 2 题错选 C
	answers := map[string]string{"1": "A", "2": "C"}
	got := ScoreExam(questions, answers)
	if got.AutoScore != 5 {
		t.Errorf("单选：期望得分 5，实际 %d", got.AutoScore)
	}
	if len(got.WrongIDs) != 1 || got.WrongIDs[0] != 2 {
		t.Errorf("单选：期望错题 ID=[2]，实际 %v", got.WrongIDs)
	}
}

func TestScoreExam_Judge(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		q(1, "judge", "true", 5),
	}
	// 大小写不敏感
	answers := map[string]string{"1": "TRUE"}
	got := ScoreExam(questions, answers)
	if got.AutoScore != 5 {
		t.Errorf("判断：期望得分 5（大小写不敏感），实际 %d", got.AutoScore)
	}
}

func TestScoreExam_Multiple(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		q(1, "multiple", "A,C", 10),
	}
	cases := []struct {
		name    string
		pick    string
		wantHit bool
	}{
		{"顺序不同但集合相同", "C,A", true},
		{"少选一项", "A", false},
		{"多选一项", "A,B,C", false},
		{"完全正确带空格", "A, C", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScoreExam(questions, map[string]string{"1": c.pick})
			if c.wantHit && got.AutoScore != 10 {
				t.Errorf("%s：期望得分 10，实际 %d", c.name, got.AutoScore)
			}
			if !c.wantHit && got.AutoScore != 0 {
				t.Errorf("%s：期望得分 0，实际 %d", c.name, got.AutoScore)
			}
		})
	}
}

func TestScoreExam_EssaySkipped(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		q(1, "essay", "", 20), // 主观题无答案
		q(2, "single", "A", 5),
	}
	answers := map[string]string{"1": "我写的答案", "2": "A"}
	got := ScoreExam(questions, answers)
	if got.AutoScore != 5 {
		t.Errorf("主观题应跳过，期望得分 5，实际 %d", got.AutoScore)
	}
}

func TestScoreExam_Unanswered(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		q(1, "single", "A", 5),
		q(2, "single", "B", 5),
	}
	// 只答了第 1 题
	answers := map[string]string{"1": "A"}
	got := ScoreExam(questions, answers)
	if got.AutoScore != 5 {
		t.Errorf("未答不计分：期望 5，实际 %d", got.AutoScore)
	}
	if len(got.UnansweredIDs) != 1 || got.UnansweredIDs[0] != 2 {
		t.Errorf("未答 ID 期望 [2]，实际 %v", got.UnansweredIDs)
	}
}

func TestSplitChoice(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"A,C,B", []string{"A", "C", "B"}},
		{`["A","C"]`, []string{"A", "C"}},
		{" A , C ", []string{"A", "C"}},
	}
	for _, c := range cases {
		got := splitChoice(c.in)
		if len(got) != len(c.want) {
			t.Errorf("splitChoice(%q) 长度期望 %d，实际 %d (%v)", c.in, len(c.want), len(got), got)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("splitChoice(%q)[%d] 期望 %q，实际 %q", c.in, i, c.want[i], got[i])
			}
		}
	}
}
