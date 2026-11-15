package model

import (
	"encoding/json"
	"testing"
)

// 逐题评分要原样以对象下发：前端直接当 map 用，不能再 parse 一次字符串。
func TestSubmissionRowMarshalQuestionScores(t *testing.T) {
	row := &SubmissionRow{Id: 1, QuestionScores: `{"12":6,"13":8}`}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	scores, ok := got["questionScores"].(map[string]interface{})
	if !ok {
		t.Fatalf("questionScores 应为对象，实际 %T: %v", got["questionScores"], got["questionScores"])
	}
	if scores["12"] != float64(6) || scores["13"] != float64(8) {
		t.Fatalf("逐题分不一致: %v", scores)
	}
}

// 没存过（空串）/ 脏数据一律给 null：前端据此判断"能不能用 AI 分回填"。
// 脏数据不能让整份答卷的响应挂掉。
func TestSubmissionRowMarshalEmptyQuestionScores(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"空串", ""},
		{"脏数据", "{not json"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(&SubmissionRow{QuestionScores: tc.raw})
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if string(got["questionScores"]) != "null" {
				t.Fatalf("期望 null，实际 %s", got["questionScores"])
			}
		})
	}
}

func TestExamSubmissionRowMarshalQuestionScores(t *testing.T) {
	row := &ExamSubmissionRow{Id: 2, QuestionScores: `{"21":8}`}
	b, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(got["questionScores"]) != `{"21":8}` {
		t.Fatalf("逐题分未下发: %s", got["questionScores"])
	}
}
