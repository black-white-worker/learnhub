package class

import (
	"encoding/json"
	"testing"
)

func TestMarshalQuestionScores(t *testing.T) {
	// 空 map 存空串：前端靠它区分"没批过逐题"和"批了全 0 分"
	got, err := MarshalQuestionScores(nil)
	if err != nil || got != "" {
		t.Fatalf("空 map 期望空串，实际 %q / %v", got, err)
	}
	got, err = MarshalQuestionScores(map[string]float64{"12": 6, "13": 8.5})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]float64
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if back["12"] != 6 || back["13"] != 8.5 {
		t.Fatalf("逐题分不一致: %v", back)
	}
}

func TestMarshalQuestionScoresRejectsNegative(t *testing.T) {
	if _, err := MarshalQuestionScores(map[string]float64{"12": -1}); err == nil {
		t.Fatal("负分应被拒绝")
	} else if be, ok := err.(*BizError); !ok || be.Code != 400 {
		t.Fatalf("期望 400 业务错误，实际 %T %v", err, err)
	}
}
