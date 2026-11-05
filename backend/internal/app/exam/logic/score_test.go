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
	// 正确答案 A,C 共 2 个，满分 10 分
	questions := []*model.ExamQuestionRow{
		q(1, "multiple", "A,C", 10),
	}
	cases := []struct {
		name    string
		pick    string
		wantRaw float64 // 期望原始得分（未四舍五入）
		wantRnd int     // 期望四舍五入后总分
	}{
		{"顺序不同但集合相同", "C,A", 10, 10},
		{"少选一项 A", "A", 5, 5},       // 1/2 × 10 = 5
		{"错选 B（多选）", "A,B,C", 0, 0}, // 选了 B 不在正确集合 → 0
		{"完全正确带空格", "A, C", 10, 10},
		{"只选 C", "C", 5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScoreExam(questions, map[string]string{"1": c.pick})
			if got.PerQuestion[1] != c.wantRaw {
				t.Errorf("%s：每题得分期望 %v，实际 %v", c.name, c.wantRaw, got.PerQuestion[1])
			}
			if got.AutoScore != c.wantRnd {
				t.Errorf("%s：总分期望 %d，实际 %d", c.name, c.wantRnd, got.AutoScore)
			}
		})
	}
}

func TestScoreExam_MultipleThreeCorrect(t *testing.T) {
	// 用户举例：5 个选项里正确答案 A,C,D 共 3 个，满分 5 分
	questions := []*model.ExamQuestionRow{
		q(1, "multiple", "A,C,D", 5),
	}
	cases := []struct {
		name    string
		pick    string
		wantRaw float64
	}{
		{"全对 3/3", "A,C,D", 5},
		{"对 2 个漏 1 个", "A,C", 5 * 2.0 / 3.0},
		{"对 1 个", "A", 5 * 1.0 / 3.0},
		{"错选 B", "A,B,C,D", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ScoreExam(questions, map[string]string{"1": c.pick})
			if got.PerQuestion[1] != c.wantRaw {
				t.Errorf("%s：期望每题得分 %v，实际 %v", c.name, c.wantRaw, got.PerQuestion[1])
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
		{"A,C,B", []string{"A", "B", "C"}}, // splitChoice 内部会排序
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
