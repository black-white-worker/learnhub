package logic

import (
	"testing"
)

// 登录限流的核心判定与 key 生成，单独锁住，避免以后被"优化"掉。
func TestLoginFailKey(t *testing.T) {
	if got := LoginFailKey("1.2.3.4", "Admin"); got != "login:fail:1.2.3.4:admin" {
		t.Fatalf("key 格式不对: %s", got)
	}
	// 用户名大小写不应绕过限流
	if LoginFailKey("1.2.3.4", "ADMIN") != LoginFailKey("1.2.3.4", "admin") {
		t.Fatal("大小写不同的用户名必须落到同一个 key")
	}
	// 不同 IP 互不影响
	if LoginFailKey("1.2.3.4", "admin") == LoginFailKey("5.6.7.8", "admin") {
		t.Fatal("不同 IP 必须分开计数")
	}
	// 空 IP 用 unknown 兜底，不能生成 login:fail::admin 这种歧义 key
	if got := LoginFailKey("", "admin"); got != "login:fail:unknown:admin" {
		t.Fatalf("空 IP 应兜底为 unknown，实际 %s", got)
	}
}

func TestLoginLocked(t *testing.T) {
	cases := map[int]bool{0: false, 4: false, 5: true, 6: true}
	for fails, want := range cases {
		if got := LoginLocked(fails); got != want {
			t.Fatalf("LoginLocked(%d)=%v，期望 %v", fails, got, want)
		}
	}
}

// 没有 Redis（nil 守卫 / nil 连接）时必须放行，不能把所有人挡在门外。
func TestLoginGuard_NilSafe(t *testing.T) {
	var g *LoginGuard
	if g.Locked("1.2.3.4", "admin") {
		t.Fatal("nil 守卫不应锁定")
	}
	g.Fail("1.2.3.4", "admin")    // 不应 panic
	g.Success("1.2.3.4", "admin") // 不应 panic
	if NewLoginGuard(nil).Locked("1.2.3.4", "admin") {
		t.Fatal("没有 Redis 时不应锁定")
	}
}
