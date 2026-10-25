package class

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"learningplatform/internal/model"
)

// ===== V10 签到 / 邀请码：rand6Code =====

// 用例：rand6Code 必须是 6 位纯数字字符串
func TestRand6Code_Format(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 200; i++ {
		c := Rand6Code()
		if len(c) != 6 {
			t.Fatalf("rand6Code 长度应为 6，实际 %d: %s", len(c), c)
		}
		for _, ch := range c {
			if ch < '0' || ch > '9' {
				t.Fatalf("rand6Code 含非数字字符: %s", c)
			}
		}
		seen[c] = struct{}{}
	}
	// 200 次调用重复率应极低，粗略验证随机性（不强制唯一，避免偶发 flaky）
	if len(seen) < 190 {
		t.Fatalf("rand6Code 随机性不足，200 次只产出 %d 个不同值", len(seen))
	}
}

// ===== 路径参数解析：parseIntPath =====

// 用例：fallback 分支从 URL path 里找第一个纯数字段
func TestParseIntPath_Fallback(t *testing.T) {
	u, _ := url.Parse("http://localhost/api/class/42/announcements")
	req := &http.Request{URL: u}
	if got := ParseIntPath(req, "id"); got != 42 {
		t.Fatalf("期望解析出 42，实际 %d", got)
	}
}

// 用例：path 里没有数字段，应返回 0
func TestParseIntPath_NoNumber(t *testing.T) {
	u, _ := url.Parse("http://localhost/api/class/announcements")
	req := &http.Request{URL: u}
	if got := ParseIntPath(req, "id"); got != 0 {
		t.Fatalf("无数字段应返回 0，实际 %d", got)
	}
}

// ===== V11 作业客观题：gradeAutoAnswers =====

func newQ(id, score int64, answer string) *model.QuestionRow {
	return &model.QuestionRow{Id: id, Score: int(score), Answer: answer, Stem: "q"}
}

// 用例：全部答对 → 累计所有题分
func TestGradeAutoAnswers_AllCorrect(t *testing.T) {
	qs := []*model.QuestionRow{
		newQ(1, 5, "A"),
		newQ(2, 5, "B"),
		newQ(3, 10, "true"),
	}
	ans := map[string]string{"1": "A", "2": "B", "3": "true"}
	if got := GradeAutoAnswers(qs, ans); got != 20 {
		t.Fatalf("全对应得 20，实际 %d", got)
	}
}

// 用例：答错、未答 → 不得分
func TestGradeAutoAnswers_WrongOrMissing(t *testing.T) {
	qs := []*model.QuestionRow{
		newQ(1, 5, "A"),
		newQ(2, 5, "B"),
		newQ(3, 10, "C"),
	}
	// 第1题对，第2题错，第3题没答
	ans := map[string]string{"1": "A", "2": "D"}
	if got := GradeAutoAnswers(qs, ans); got != 5 {
		t.Fatalf("只对第1题应得 5，实际 %d", got)
	}
}

// 用例：大小写不敏感（学生答 a，答案 A 应判对）
func TestGradeAutoAnswers_CaseInsensitive(t *testing.T) {
	qs := []*model.QuestionRow{newQ(1, 5, "A")}
	ans := map[string]string{"1": "a"}
	if got := GradeAutoAnswers(qs, ans); got != 5 {
		t.Fatalf("大小写不敏感应得 5，实际 %d", got)
	}
}

// 用例：空答案 map → 0 分，不 panic
func TestGradeAutoAnswers_Empty(t *testing.T) {
	qs := []*model.QuestionRow{newQ(1, 5, "A")}
	if got := GradeAutoAnswers(qs, map[string]string{}); got != 0 {
		t.Fatalf("空答案应得 0，实际 %d", got)
	}
}

// ===== 作业提交非空校验：validateSubmissionInput =====

// 用例：全空 → 报错
func TestValidateSubmission_AllEmpty(t *testing.T) {
	if err := ValidateSubmissionInput("", "", nil); err == nil {
		t.Fatal("全空应报错")
	}
}

// 用例：纯空格也算空
func TestValidateSubmission_WhitespaceOnly(t *testing.T) {
	if err := ValidateSubmissionInput("   \n  ", "", map[string]string{}); err == nil {
		t.Fatal("纯空白文本应报错")
	}
}

// 用例：只填文本 → 通过
func TestValidateSubmission_TextOnly(t *testing.T) {
	if err := ValidateSubmissionInput("这是我的作业", "", nil); err != nil {
		t.Fatalf("只填文本应通过，实际报错: %v", err)
	}
}

// 用例：只传附件 → 通过
func TestValidateSubmission_FileOnly(t *testing.T) {
	if err := ValidateSubmissionInput("", "http://minio/assignment/x.pdf", nil); err != nil {
		t.Fatalf("只传附件应通过，实际报错: %v", err)
	}
}

// 用例：只答客观题 → 通过
func TestValidateSubmission_AnswersOnly(t *testing.T) {
	if err := ValidateSubmissionInput("", "", map[string]string{"1": "A"}); err != nil {
		t.Fatalf("只答客观题应通过，实际报错: %v", err)
	}
}

// ===== AssignmentRow.Deadline JSON 序列化 =====

// 用例：未设截止时间 → deadline 输出空字符串（前端 || 判为 falsy，显示"无截止时间"）
func TestAssignmentMarshal_NoDeadline(t *testing.T) {
	a := &model.AssignmentRow{Id: 1, Title: "hw1", Deadline: sql.NullTime{Valid: false}}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	dl, ok := m["deadline"].(string)
	if !ok {
		t.Fatalf("deadline 应为字符串，实际 %v (原始: %s)", m["deadline"], string(b))
	}
	if dl != "" {
		t.Fatalf("未设截止时间应为空串，实际 %q", dl)
	}
}

// 用例：设了截止时间 → 输出 "YYYY-MM-DD HH:mm" 格式字符串，而不是 {Time,Valid} 对象
func TestAssignmentMarshal_WithDeadline(t *testing.T) {
	tm := time.Date(2026, 10, 25, 20, 0, 0, 0, time.UTC)
	a := &model.AssignmentRow{Id: 1, Title: "hw1", Deadline: sql.NullTime{Time: tm, Valid: true}}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	s := string(b)
	// 不能再出现 {Time:...Valid:...} 这种结构
	if strings.Contains(s, `"Valid"`) {
		t.Fatalf("序列化结果仍暴露 sql.NullTime 内部结构: %s", s)
	}
	if !strings.Contains(s, "2026-10-25 20:00") {
		t.Fatalf("序列化结果应包含格式化后的截止时间, 实际: %s", s)
	}
}
