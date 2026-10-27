package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 前端传了 X-Request-ID：应该原样透传，不重新生成。
func TestRequestID_Passthrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if got := RequestID(r.Context()); got != "my-trace-123" {
			t.Errorf("context 里 request_id 期望 my-trace-123，实际 %q", got)
		}
	})
	h := RequestIDMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set(RequestIDHeader, "my-trace-123")
	w := httptest.NewRecorder()
	h(w, req)

	if !called {
		t.Fatal("next handler 没被调用")
	}
	if w.Header().Get(RequestIDHeader) != "my-trace-123" {
		t.Errorf("response header 期望原样透传，实际 %q", w.Header().Get(RequestIDHeader))
	}
}

// 前端没传：应该生成一个新的 UUID 并写回 response header。
func TestRequestID_Generate(t *testing.T) {
	var ctxRID string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxRID = RequestID(r.Context())
	})
	h := RequestIDMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if ctxRID == "" {
		t.Error("context 里 request_id 不应为空")
	}
	if got := w.Header().Get(RequestIDHeader); got != ctxRID {
		t.Errorf("response header %q 应与 context %q 一致", got, ctxRID)
	}
	if len(ctxRID) != 36 {
		t.Errorf("UUID 长度期望 36，实际 %d (%q)", len(ctxRID), ctxRID)
	}
}
