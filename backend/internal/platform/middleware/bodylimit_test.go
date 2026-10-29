package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newBodyLimit 造一个 1KB 默认上限 + 上传路由放开的中间件。
func newBodyLimit() BodyLimit {
	return BodyLimit{
		Default: 1024,
		Overrides: []BodyLimitRule{
			{PathPrefix: "/api/course/upload/", Limit: 1 << 20},
		},
	}
}

// 普通接口超过默认上限：直接 413，且不再进入业务 handler。
func TestBodyLimit_RejectsOversizeJSON(t *testing.T) {
	called := false
	h := newBodyLimit().Handle(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	body := strings.Repeat("a", 2048)
	req := httptest.NewRequest(http.MethodPost, "/api/class/1/exam", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h(rec, req)

	if called {
		t.Fatal("超限请求不应进入业务 handler")
	}
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("期望 413，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "请求体过大") {
		t.Fatalf("错误体应为统一信封，实际 %s", rec.Body.String())
	}
}

// 上传路由被显式放宽：同样的 body 可以通过。
func TestBodyLimit_AllowsUploadOverride(t *testing.T) {
	h := newBodyLimit().Handle(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("读 body 失败: %v", err)
		}
		if len(b) != 2048 {
			t.Errorf("期望读到完整 2048 字节，实际 %d", len(b))
		}
		w.WriteHeader(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/course/upload/videos", strings.NewReader(strings.Repeat("a", 2048)))
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("上传路由应放行，实际 %d", rec.Code)
	}
}

// 未知长度（chunked）时依靠 MaxBytesReader 在读取阶段截断。
func TestBodyLimit_ChunkedBodyStopsAtLimit(t *testing.T) {
	h := newBodyLimit().Handle(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if !BodyTooLarge(err) {
			t.Errorf("期望识别为 body 超限，实际 err=%v", err)
		}
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/class/1/exam", bytes.NewReader(bytes.Repeat([]byte("a"), 4096)))
	req.ContentLength = -1 // 模拟 chunked，前置的 Content-Length 预检查不会生效
	rec := httptest.NewRecorder()
	h(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handler 应正常返回（错误由业务层翻译），实际 %d", rec.Code)
	}
}

// limitFor：默认值、精确前缀、非前缀路径。
func TestBodyLimit_LimitFor(t *testing.T) {
	b := newBodyLimit()
	if got := b.limitFor("/api/course/list"); got != 1024 {
		t.Fatalf("默认上限应为 1024，实际 %d", got)
	}
	if got := b.limitFor("/api/course/upload/videos"); got != 1<<20 {
		t.Fatalf("上传路由上限应为 1MB，实际 %d", got)
	}
	if got := b.limitFor("/api/assignment/uploadx"); got != 1024 {
		t.Fatalf("非前缀路径不应命中，实际 %d", got)
	}
}

// 零值 BodyLimit 也必须可用（默认 1MB）。
func TestBodyLimit_ZeroValue(t *testing.T) {
	var b BodyLimit
	if got := b.limitFor("/api/x"); got != DefaultBodyLimit {
		t.Fatalf("零值应回退到 %d，实际 %d", DefaultBodyLimit, got)
	}
}
