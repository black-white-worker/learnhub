package logic

// BuildExamGradeItems 的回归测试：考试主观题送 AI 批改的载荷规则。
// 纯函数测试，不依赖数据库 —— 与 class 包 ai_grade_utils_test.go 同一套口径。
import (
	"strconv"
	"testing"

	"learningplatform/internal/model"
	"learningplatform/internal/platform/aiservice"
)

func eqItems(t *testing.T, got []aiservice.GradeItem, want []aiservice.GradeItem) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("items 数量 = %d, 期望 %d", len(got), len(want))
	}
	for i := range want {
		if got[i].QuestionId != want[i].QuestionId ||
			got[i].MaxScore != want[i].MaxScore ||
			got[i].StudentAnswer != want[i].StudentAnswer ||
			got[i].Stem != want[i].Stem {
			t.Fatalf("items[%d] = %+v, 期望 %+v", i, got[i], want[i])
		}
	}
}

func TestBuildExamGradeItems(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		{Id: 101, Type: "single", Stem: "单选题", Score: 2},
		{Id: 102, Type: "essay", Stem: "简答：光电效应", Answer: "爱因斯坦方程", Score: 10},
		{Id: 103, Type: "judge", Stem: "判断题", Score: 2},
		{Id: 104, Type: "essay", Stem: "简答：不确定关系", Score: 0}, // 分值没配 → 兜底 100
		{Id: 105, Type: "fileupload", Stem: "附件题", Score: 20},
	}
	answers := map[string]string{
		"101": "A", // 客观题不送
		"102": "由 hν = W + eU 可得……", // 主观题已作答
		"104": "",                    // 主观题未作答 → 跳过
		// 105 fileupload 不送
	}
	got := BuildExamGradeItems(questions, answers)
	want := []aiservice.GradeItem{
		{QuestionId: 102, Stem: "简答：光电效应", StudentAnswer: "由 hν = W + eU 可得……", StandardAnswer: "爱因斯坦方程", MaxScore: 10},
	}
	eqItems(t, got, want)
}

func TestBuildExamGradeItemsCapsAtMaxItems(t *testing.T) {
	// 超过 30 道主观题时截断到 30：ai-service 那边超了会静默截断，
	// 这边截断才能保证「批了几道」和「建议表满分」口径一致。
	questions := make([]*model.ExamQuestionRow, 0, 35)
	for i := 0; i < 35; i++ {
		questions = append(questions, &model.ExamQuestionRow{Id: int64(1000 + i), Type: "essay", Stem: "题", Score: 1})
	}
	answers := map[string]string{}
	for i := 0; i < 35; i++ {
		answers[strconv.FormatInt(int64(1000+i), 10)] = "答"
	}
	got := BuildExamGradeItems(questions, answers)
	if len(got) != 30 {
		t.Fatalf("items 数量 = %d, 期望截断到 30", len(got))
	}
}
