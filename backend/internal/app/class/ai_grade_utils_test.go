package class

import (
	"fmt"
	"strings"
	"testing"

	"learningplatform/internal/model"
)

func essayQ(id int64, score int, answer string) *model.QuestionRow {
	return &model.QuestionRow{Id: id, Type: "essay", Stem: fmt.Sprintf("第%d题", id), Score: score, Answer: answer}
}

func objQ(id int64, typ string, score int) *model.QuestionRow {
	return &model.QuestionRow{Id: id, Type: typ, Stem: "客观题", Score: score, Answer: "A"}
}

// 用例：只有 essay 进批改载荷，单选/多选/填空/附件都不送
func TestBuildGradeItems_OnlyEssay(t *testing.T) {
	qs := []*model.QuestionRow{
		objQ(1, "single", 5),
		essayQ(2, 10, "参考答案"),
		objQ(3, "fill", 5),
		objQ(4, "fileupload", 10),
		essayQ(5, 20, ""),
	}
	answers := map[string]string{"1": "A", "2": "学生答案2", "4": "http://x/file.pdf", "5": "学生答案5"}
	items := BuildGradeItems(qs, answers, "")
	if len(items) != 2 {
		t.Fatalf("期望 2 道主观题，实际 %d: %+v", len(items), items)
	}
	if items[0].QuestionId != 2 || items[1].QuestionId != 5 {
		t.Fatalf("题号顺序错了: %d, %d", items[0].QuestionId, items[1].QuestionId)
	}
	if items[0].MaxScore != 10 || items[1].MaxScore != 20 {
		t.Fatalf("分值没跟着题目走: %d, %d", items[0].MaxScore, items[1].MaxScore)
	}
	if items[0].StandardAnswer != "参考答案" {
		t.Fatalf("参考答案应取题目 Answer，实际 %q", items[0].StandardAnswer)
	}
}

// 用例：学生没作答的主观题直接跳过，不让 AI 打个 0 分白烧额度
func TestBuildGradeItems_SkipsBlankAnswers(t *testing.T) {
	qs := []*model.QuestionRow{essayQ(1, 10, ""), essayQ(2, 10, ""), essayQ(3, 10, "")}
	answers := map[string]string{"1": "写了", "2": "   ", "3": ""}
	items := BuildGradeItems(qs, answers, "整份的正文，但不该分给具体题")
	if len(items) != 1 || items[0].QuestionId != 1 {
		t.Fatalf("只应保留第 1 题，实际 %+v", items)
	}
}

// 用例：整份只有一道主观题且没写答案时，退回 content 正文（老交法）
func TestBuildGradeItems_ContentFallback(t *testing.T) {
	single := []*model.QuestionRow{essayQ(1, 10, "")}
	items := BuildGradeItems(single, map[string]string{}, "  正文里写的答案  ")
	if len(items) != 1 || items[0].StudentAnswer != "正文里写的答案" {
		t.Fatalf("单题应交回 content，实际 %+v", items)
	}

	// 多题时不能把全文塞给每一题，否则每道题都拿到同一篇正文，分数全乱
	two := []*model.QuestionRow{essayQ(1, 10, ""), essayQ(2, 10, "")}
	if got := BuildGradeItems(two, map[string]string{}, "正文"); len(got) != 0 {
		t.Fatalf("多题时不应退回 content，实际 %+v", got)
	}
}

// 用例：题目没配分值时用默认满分，不能给 0（LLM 按满分打分，0 会算不出分）
func TestBuildGradeItems_DefaultMaxScore(t *testing.T) {
	items := BuildGradeItems([]*model.QuestionRow{essayQ(1, 0, "")}, map[string]string{"1": "答"}, "")
	if len(items) != 1 || items[0].MaxScore != DefaultAiGradeMaxScore {
		t.Fatalf("期望默认满分 %d，实际 %+v", DefaultAiGradeMaxScore, items)
	}
}

// 用例：超过上限只留前 30 道（和 ai-service 的 MAX_ITEMS 对齐）
func TestBuildGradeItems_CapsAtMaxItems(t *testing.T) {
	qs := make([]*model.QuestionRow, 0, 35)
	answers := map[string]string{}
	for i := 1; i <= 35; i++ {
		q := essayQ(int64(i), 2, "")
		qs = append(qs, q)
		answers[fmt.Sprintf("%d", i)] = "答案"
	}
	items := BuildGradeItems(qs, answers, "")
	if len(items) != MaxAiGradeItems {
		t.Fatalf("应截断到 %d 道，实际 %d", MaxAiGradeItems, len(items))
	}
	if items[MaxAiGradeItems-1].QuestionId != int64(MaxAiGradeItems) {
		t.Fatalf("截断要保留前 30 道，末题实际 %d", items[MaxAiGradeItems-1].QuestionId)
	}
}

// 用例：answers 为空 / nil 都不能 panic
func TestBuildGradeItems_NilAnswers(t *testing.T) {
	if got := BuildGradeItems([]*model.QuestionRow{essayQ(1, 10, "")}, nil, ""); len(got) != 0 {
		t.Fatalf("nil answers 且无正文时应返回空，实际 %+v", got)
	}
}

// 用例：EssayQuestions 只挑主观题，且能容忍 nil 行
func TestEssayQuestions_FilterAndNilSafe(t *testing.T) {
	qs := []*model.QuestionRow{nil, objQ(1, "single", 5), essayQ(2, 10, ""), nil, essayQ(3, 10, "")}
	got := EssayQuestions(qs)
	if len(got) != 2 || got[0].Id != 2 || got[1].Id != 3 {
		t.Fatalf("应挑出 2、3 题，实际 %+v", got)
	}
}

// 用例：状态文案要全覆盖，前端兜底显示用
func TestAiGradeStatusText(t *testing.T) {
	cases := map[string]string{
		"":                      "未发起",
		model.AIGradeProcessing: "AI 批改中",
		model.AIGradeDone:       "AI 初评完成",
		model.AIGradeManual:     "AI 未能批改，请人工批改",
		model.AIGradeFailed:     "AI 批改失败",
	}
	for status, want := range cases {
		if got := AiGradeStatusText(status); got != want {
			t.Fatalf("状态 %q 期望文案 %q，实际 %q", status, want, got)
		}
	}
	if got := AiGradeStatusText("weird"); got != "weird" {
		t.Fatalf("未知状态应原样返回，实际 %q", got)
	}
}

// 用例：载荷里不能出现题目 id 之外的键（answers 的 key 是字符串 id）
func TestBuildGradeItems_AnswerKeyFormat(t *testing.T) {
	qs := []*model.QuestionRow{essayQ(12, 10, "")}
	if got := BuildGradeItems(qs, map[string]string{"12": "命中"}, ""); len(got) != 1 || !strings.Contains(got[0].StudentAnswer, "命中") {
		t.Fatalf("answers 应以题目 id 字符串为 key，实际 %+v", got)
	}
	if got := BuildGradeItems(qs, map[string]string{"11": "错位"}, ""); len(got) != 0 {
		t.Fatalf("key 对不上时不应命中，实际 %+v", got)
	}
}
