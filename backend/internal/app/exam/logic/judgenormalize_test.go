package logic

import (
	"testing"

	"learningplatform/internal/model"
)

// 判断题历史数据兼容：
// 早期题库把"对/错"存成 A/B，新版出题器存 true/false，学生端提交 true/false。
// 不做归一化的话，老题目的判断题会永远判错 —— 正是"学生选对却不得分"的一种成因。
func TestNormalizeJudge(t *testing.T) {
	trues := []string{"true", "TRUE", "A", "a", " 对 ", "√", "1", "T", "yes"}
	falses := []string{"false", "False", "B", "b", " 错 ", "×", "0", "F", "no"}
	for _, s := range trues {
		if got := normalizeJudge(s); got != "true" {
			t.Fatalf("normalizeJudge(%q)=%q，期望 true", s, got)
		}
	}
	for _, s := range falses {
		if got := normalizeJudge(s); got != "false" {
			t.Fatalf("normalizeJudge(%q)=%q，期望 false", s, got)
		}
	}
	// 无法识别的输入原样返回（小写），交由调用方比较
	if got := normalizeJudge("unknown"); got != "unknown" {
		t.Fatalf("未知输入应原样返回，实际 %q", got)
	}
}

func TestScoreExam_JudgeLegacyAnswerAB(t *testing.T) {
	// 老题库：正确答案存 A，学生端提交 true
	q := &model.ExamQuestionRow{Id: 1, Type: "judge", Answer: "A", Score: 10}
	res := ScoreExam([]*model.ExamQuestionRow{q}, map[string]string{"1": "true"})
	if res.AutoScore != 10 {
		t.Fatalf("老题 A + 学生 true 应得 10，实际 %d（明细 %v）", res.AutoScore, res.PerQuestion)
	}
	if len(res.WrongIDs) != 0 {
		t.Fatalf("不应计入错题，实际 %v", res.WrongIDs)
	}
}

func TestScoreExam_JudgeLegacyAnswerB(t *testing.T) {
	q := &model.ExamQuestionRow{Id: 2, Type: "judge", Answer: "B", Score: 10}
	res := ScoreExam([]*model.ExamQuestionRow{q}, map[string]string{"2": "false"})
	if res.AutoScore != 10 {
		t.Fatalf("老题 B + 学生 false 应得 10，实际 %d", res.AutoScore)
	}
}

func TestScoreExam_JudgeMismatchStillZero(t *testing.T) {
	q := &model.ExamQuestionRow{Id: 3, Type: "judge", Answer: "true", Score: 10}
	res := ScoreExam([]*model.ExamQuestionRow{q}, map[string]string{"3": "false"})
	if res.AutoScore != 0 {
		t.Fatalf("选错应得 0，实际 %d", res.AutoScore)
	}
	if len(res.WrongIDs) != 1 {
		t.Fatalf("应计入 1 道错题，实际 %v", res.WrongIDs)
	}
}

// 单选题不能被判断题的归一化误伤：A 仍然是 A，不能变成 true。
func TestScoreExam_SingleNotNormalized(t *testing.T) {
	q := &model.ExamQuestionRow{Id: 4, Type: "single", Answer: "B", Score: 10}
	// 学生选 A（错）：如果单选题也被归一化，A→true、B→false，仍然不等，结果一样；
	// 但 选 true 不应被判对（只有判断题才做归一化）
	res := ScoreExam([]*model.ExamQuestionRow{q}, map[string]string{"4": "true"})
	if res.AutoScore != 0 {
		t.Fatalf("单选题提交 true 不应得分，实际 %d", res.AutoScore)
	}
}
