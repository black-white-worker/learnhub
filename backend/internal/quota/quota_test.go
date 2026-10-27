package quota

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// 核心判定：上限内的放行、超限的拒绝、未配置上限的永远放行。
func TestAllowed(t *testing.T) {
	const MB = 1 << 20
	cases := []struct {
		name           string
		used, add, lim int64
		want           bool
	}{
		{"空配额可传", 0, 10 * MB, 100 * MB, true},
		{"刚好用满", 90 * MB, 10 * MB, 100 * MB, true},
		{"超 1 字节", 90*MB + 1, 10 * MB, 100 * MB, false},
		{"上限为 0 表示不限制", 0, 999 * MB, 0, true},
		{"负上限也视为不限制", 0, 1 * MB, -1, true},
		{"add 为负按 0 处理", 100 * MB, -5, 100 * MB, true},
		{"used 为负按 0 处理", -10, 5 * MB, 5 * MB, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Allowed(c.used, c.add, c.lim); got != c.want {
				t.Fatalf("Allowed(%d,%d,%d)=%v，期望 %v", c.used, c.add, c.lim, got, c.want)
			}
		})
	}
}

// key 必须带日期与用户 id：日期保证自然清零，用户 id 保证互不影响。
func TestQuotaKey(t *testing.T) {
	now := time.Date(2026, 10, 27, 23, 59, 0, 0, time.Local)
	got := QuotaKey(42, now)
	if got != "upload:quota:20261027:42" {
		t.Fatalf("key 格式不对: %s", got)
	}
	if QuotaKey(43, now) == got {
		t.Fatal("不同用户必须用不同 key")
	}
	// 跨天必须换 key（否则永远不会清零）
	next := now.Add(2 * time.Minute)
	if QuotaKey(42, next) == got {
		t.Fatal("跨天应换 key")
	}
}

// 未启用（没配上限 / 没有 Redis）时必须放行，不能因为配额配置缺失挡住上传。
func TestReserve_DisabledPassthrough(t *testing.T) {
	if err := New(nil, 100).Reserve(1, 200); err != nil {
		t.Fatalf("没有 Redis 时应放行，实际: %v", err)
	}
	if err := New(nil, 0).Reserve(1, 200); err != nil {
		t.Fatalf("未配置上限时应放行，实际: %v", err)
	}
	var nilLimiter *Limiter
	if err := nilLimiter.Reserve(1, 200); err != nil {
		t.Fatalf("nil Limiter 应放行，实际: %v", err)
	}
	if nilLimiter.Enabled() {
		t.Fatal("nil Limiter 不应处于启用状态")
	}
	if nilLimiter.Limit() != 0 || nilLimiter.Used(1) != 0 {
		t.Fatal("nil Limiter 的 Limit/Used 应返回 0")
	}
}

// 超限错误要带 413，前端据此提示"额度不足"而不是"服务器错误"。
func TestExceeded_StatusAndMessage(t *testing.T) {
	const MB = 1 << 20
	e := &Exceeded{Limit: 100 * MB, Used: 95 * MB, Add: 10 * MB}
	if e.StatusCode() != http.StatusRequestEntityTooLarge {
		t.Fatalf("状态码应为 413，实际 %d", e.StatusCode())
	}
	msg := e.Error()
	for _, want := range []string{"额度不足", "95.0 MB", "100.0 MB", "10.0 MB"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("错误文案缺少 %q: %s", want, msg)
		}
	}
}
