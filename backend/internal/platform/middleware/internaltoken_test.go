package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// passThrough 记录业务 handler 有没有真的被执行。
func passThrough(called *bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		*called = true
		w.WriteHeader(http.StatusOK)
	}
}

// 用例：令牌对得上才放行
func TestInternalToken_MatchingTokenPasses(t *testing.T) {
	called := false
	h := NewInternalTokenMiddleware("s3cr3t").Handle(passThrough(&called))

	req := httptest.NewRequest(http.MethodPost, "/api/assignment/submission/9/ai-callback", nil)
	req.Header.Set(InternalTokenHeader, "s3cr3t")
	rec := httptest.NewRecorder()
	h(rec, req)

	if !called {
		t.Fatal("令牌正确时应进入业务 handler")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", rec.Code)
	}
}

// 用例：令牌不对 → 403，且不进业务 handler
func TestInternalToken_WrongTokenRejected(t *testing.T) {
	for name, token := range map[string]string{
		"空值":      "",
		"别的值":     "not-the-token",
		"少一位":     "s3cr3",
		"多一位（前缀）": "s3cr3tx",
	} {
		called := false
		h := NewInternalTokenMiddleware("s3cr3t").Handle(passThrough(&called))
		req := httptest.NewRequest(http.MethodPost, "/api/assignment/submission/9/ai-callback", nil)
		if token != "" {
			req.Header.Set(InternalTokenHeader, token)
		}
		rec := httptest.NewRecorder()
		h(rec, req)

		if called {
			t.Fatalf("%s：不该放行", name)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s：期望 403，实际 %d", name, rec.Code)
		}
		// 必须是 JSON 信封，worker 用 resp.status_code==200 判断，拿到 HTML 会误判
		var env map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s：响应不是 JSON 信封: %s", name, rec.Body.String())
		}
		if env["code"] != float64(http.StatusForbidden) {
			t.Fatalf("%s：信封 code 应为 403，实际 %v", name, env["code"])
		}
	}
}

// 用例：配置里令牌为空 = 未启用回调，一律拒绝。
// 绝不能因为"忘了配"就把 worker 回调路由变成公开路由 —— 任何人都能往成绩里写 AI 分数。
func TestInternalToken_EmptyConfiguredTokenRejectsAll(t *testing.T) {
	for _, headerToken := range []string{"", "anything"} {
		called := false
		h := NewInternalTokenMiddleware("").Handle(passThrough(&called))
		req := httptest.NewRequest(http.MethodPost, "/api/assignment/submission/9/ai-callback", nil)
		if headerToken != "" {
			req.Header.Set(InternalTokenHeader, headerToken)
		}
		rec := httptest.NewRecorder()
		h(rec, req)

		if called {
			t.Fatalf("未配置令牌时不能放行（带 %q 的请求）", headerToken)
		}
		if rec.Code != http.StatusForbidden {
			t.Fatalf("期望 403，实际 %d", rec.Code)
		}
	}
}

// 用例：InternalUserID 只在拿到正整数时才算有效
func TestInternalUserID(t *testing.T) {
	cases := []struct {
		raw  string
		want int64
	}{
		{"7", 7},
		{"0", 0},
		{"-1", 0},
		{"", 0},
		{"abc", 0},
		{"9223372036854775808", 0}, // 溢出 int64
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/api/ai/task/t1", nil)
		if c.raw != "" {
			req.Header.Set(InternalUserIDHeader, c.raw)
		}
		if got := InternalUserID(req); got != c.want {
			t.Fatalf("X-User-Id=%q 期望 %d，实际 %d", c.raw, c.want, got)
		}
	}
}
