package logic

import (
	"encoding/json"
	"testing"

	"learningplatform/internal/model"
)

// 老师端「查看答卷」的每题得分是本次修复的重点，单独抽成纯函数以便回归：
// 曾经因为老师分支不返回 perQuestion，前端只能读学生的字段，导致
// 学生明明选对了、弹窗里每道题都显示"得分 0"。

func TestBuildSubmissionScores_PerSubmission(t *testing.T) {
	questions := []*model.ExamQuestionRow{
		{Id: 11, Type: "single", Answer: "B", Score: 20},
		{Id: 12, Type: "multiple", Answer: "A,B,D", Score: 20},
		{Id: 13, Type: "essay", Answer: "", Score: 20},
	}
	subs := []*model.ExamSubmissionRow{
		// 全对：两题各 20，主观题不参与自动判分
		{Id: 101, Answers: `{"11":"B","12":"A,B,D","13":"随便写点"}`},
		// 一题错选、一题没答
		{Id: 102, Answers: `{"11":"A"}`},
	}

	got := buildSubmissionScores(subs, questions)

	if _, ok := got[101]; !ok {
		t.Fatal("每份提交都应有一条得分记录")
	}
	if v := got[101][11]; v != 20 {
		t.Fatalf("101 题 11 应得 20，实际 %v", v)
	}
	if v := got[101][12]; v != 20 {
		t.Fatalf("101 题 12 应得 20，实际 %v", v)
	}
	if _, ok := got[101][13]; ok {
		t.Fatal("主观题不应出现在自动判分明细里")
	}
	if v := got[102][11]; v != 0 {
		t.Fatalf("102 题 11 选错应得 0，实际 %v", v)
	}
	if _, ok := got[102][12]; ok {
		t.Fatal("未作答的题不应有得分记录（前端据此显示未答）")
	}
}

func TestBuildSubmissionScores_EmptyAnswersDoesNotPanic(t *testing.T) {
	questions := []*model.ExamQuestionRow{{Id: 1, Type: "single", Answer: "A", Score: 5}}
	subs := []*model.ExamSubmissionRow{
		{Id: 1, Answers: ""},
		{Id: 2, Answers: "not-json"},
	}
	got := buildSubmissionScores(subs, questions)
	if len(got) != 2 {
		t.Fatalf("空/坏 JSON 也要给每条提交建一条记录，实际 %d 条", len(got))
	}
	if len(got[1]) != 0 || len(got[2]) != 0 {
		t.Fatal("空/坏 JSON 不应产生任何得分")
	}
}

// 保留 json 包的引用，防止未来有人把 helper 移走时忘记 import。
var _ = json.Marshal
