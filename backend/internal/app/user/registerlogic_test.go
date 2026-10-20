package user

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// 测试用例 1：注册一个全新用户 → 成功，返回用户名
// 注意：这是集成测试，需要 MySQL 已启动（docker compose up -d mysql）。
// 用户名带时间戳，避免重复运行测试时撞库；IP 用独立地址隔离限流计数。
func TestRegisterLogic_Success(t *testing.T) {
	ctx := context.WithValue(context.Background(), clientIPKey, "10.0.0.1")
	l := NewRegisterLogic(ctx, newTestSvcCtx())

	uname := fmt.Sprintf("zhangsan_%d", time.Now().UnixNano())
	resp, err := l.Register(&RegisterRequest{Username: uname, Password: "123456"})
	if err != nil {
		t.Fatalf("期望注册成功，却报错: %v", err)
	}
	if resp.Username != uname {
		t.Fatalf("期望返回用户名 %s，实际是 %s", uname, resp.Username)
	}
}

// 测试用例 2：用户名已存在（admin 是初始化 SQL 预置账号）→ 注册失败
func TestRegisterLogic_DuplicateUser(t *testing.T) {
	ctx := context.WithValue(context.Background(), clientIPKey, "10.0.0.2")
	l := NewRegisterLogic(ctx, newTestSvcCtx())

	_, err := l.Register(&RegisterRequest{Username: "admin", Password: "123456"})
	if err == nil {
		t.Fatal("期望用户名已存在时报错，却没有报错")
	}
}

// 测试用例 3：用户名或密码为空 → 注册失败（参数校验在限流之前）
func TestRegisterLogic_EmptyParam(t *testing.T) {
	ctx := context.WithValue(context.Background(), clientIPKey, "10.0.0.3")
	l := NewRegisterLogic(ctx, newTestSvcCtx())

	_, err := l.Register(&RegisterRequest{Username: "", Password: ""})
	if err == nil {
		t.Fatal("期望参数为空时报错，却没有报错")
	}
}

// 测试用例 4：同一 IP 一分钟内注册超过 5 次 → 触发限流拒绝
// Redis INCR 计数窗口为 60 秒，测试用独立 IP 避免影响其他用例。
func TestRegisterLogic_RateLimit(t *testing.T) {
	ctx := context.WithValue(context.Background(), clientIPKey, "10.0.0.9")
	l := NewRegisterLogic(ctx, newTestSvcCtx())

	// 1. 前 5 次允许通过（每次注册后立刻删除，避免污染数据库）
	for i := 0; i < registerLimitPerMinute; i++ {
		uname := fmt.Sprintf("ratelimit_%d_%d", time.Now().UnixNano(), i)
		resp, err := l.Register(&RegisterRequest{Username: uname, Password: "123456"})
		if err != nil {
			t.Fatalf("第 %d 次注册应成功，却报错: %v", i+1, err)
		}
		if err := newTestSvcCtx().UserStore.Delete(resp.Username); err != nil {
			t.Fatalf("清理测试用户失败: %v", err)
		}
	}

	// 2. 第 6 次应被限流拒绝
	_, err := l.Register(&RegisterRequest{
		Username: fmt.Sprintf("ratelimit_%d_6", time.Now().UnixNano()),
		Password: "123456",
	})
	if err == nil {
		t.Fatal("期望超过注册次数限制时报错，却没有报错")
	}
}
