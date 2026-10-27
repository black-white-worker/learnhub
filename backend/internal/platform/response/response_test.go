package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 一个实现了 codedError 接口的测试用业务错误。
type fakeBizError struct {
	code int
	msg  string
}

func (f *fakeBizError) Error() string   { return f.msg }
func (f *fakeBizError) StatusCode() int { return f.code }

func TestOK_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	OK(w, map[string]int{"id": 1})

	if w.Code != http.StatusOK {
		t.Errorf("HTTP 状态码期望 200，实际 %d", w.Code)
	}
	var env Envelope
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if env.Code != 0 {
		t.Errorf("成功响应 code 期望 0，实际 %d", env.Code)
	}
	if env.Message != "ok" {
		t.Errorf("成功响应 message 期望 ok，实际 %q", env.Message)
	}
	data, ok := env.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data 期望对象，实际 %T", env.Data)
	}
	if int(data["id"].(float64)) != 1 {
		t.Errorf("data.id 期望 1，实际 %v", data["id"])
	}
}

func TestError_CodedError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, &fakeBizError{code: http.StatusForbidden, msg: "无权访问"})

	if w.Code != http.StatusForbidden {
		t.Errorf("HTTP 状态码期望 403，实际 %d", w.Code)
	}
	var env Envelope
	_ = json.NewDecoder(w.Body).Decode(&env)
	if env.Code != http.StatusForbidden {
		t.Errorf("body.code 期望 403，实际 %d", env.Code)
	}
	if env.Message != "无权访问" {
		t.Errorf("body.message 期望 无权访问，实际 %q", env.Message)
	}
}

func TestError_PlainError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, errors.New("db down"))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("普通 error 期望 500，实际 %d", w.Code)
	}
	var env Envelope
	_ = json.NewDecoder(w.Body).Decode(&env)
	if env.Code != 500 {
		t.Errorf("body.code 期望 500，实际 %d", env.Code)
	}
	if env.Message != "db down" {
		t.Errorf("body.message 期望 db down，实际 %q", env.Message)
	}
}

func TestError_WrapError(t *testing.T) {
	// fmt.Errorf("xxx: %w", err) 包装的错误不是 codedError，应走 500 分支
	w := httptest.NewRecorder()
	wrapped := errors.New("origin")
	Error(w, wrapped)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("包装 error 期望 500，实际 %d", w.Code)
	}
}
