package logic

import (
	"context"
	"testing"

	"learningplatform/internal/app/user"
	"learningplatform/internal/model"
)

// 测试用例 1：登录后登出 → Redis 会话被删除，返回登出用户名
func TestLogoutLogic_Success(t *testing.T) {
	svcCtx := newTestSvcCtx()

	// 1. 先登录，拿到真实 token（会话写入 Redis）
	loginLogic := NewLoginLogic(context.Background(), svcCtx)
	loginResp, err := loginLogic.Login(&user.LoginRequest{Username: "admin", Password: "123456"})
	if err != nil {
		t.Fatalf("前置准备：登录失败: %v", err)
	}

	// 2. 确认 Redis 里已有会话
	val, err := svcCtx.Redis.Get(user.SessionKeyPrefix + loginResp.Token)
	if err != nil || val == "" {
		t.Fatal("前置准备：期望登录后 Redis 里有会话，却没有")
	}

	// 3. 模拟鉴权中间件放入 context 的 token 与会话，调用登出逻辑
	ctx := context.WithValue(context.Background(), user.AuthTokenKey,loginResp.Token)
	ctx = context.WithValue(ctx, user.AuthSessionKey, model.SessionUser{Username: "admin"})
	logoutLogic := NewLogoutLogic(ctx, svcCtx)
	out, err := logoutLogic.Logout(loginResp.Token)
	if err != nil {
		t.Fatalf("期望登出成功，却报错: %v", err)
	}
	if out.Username != "admin" {
		t.Fatalf("期望登出用户名是 admin，实际是 %s", out.Username)
	}

	// 4. Redis 会话应已被删除
	val2, err := svcCtx.Redis.Get(user.SessionKeyPrefix + loginResp.Token)
	if err == nil && val2 != "" {
		t.Fatal("期望登出后 Redis 会话被删除，却仍然存在")
	}
}

// 测试用例 2：对不存在的 token 登出 → 不报错（Redis DEL 幂等）
func TestLogoutLogic_Idempotent(t *testing.T) {
	ctx := context.WithValue(context.Background(), user.AuthTokenKey,"not-exist-token")
	ctx = context.WithValue(ctx, user.AuthSessionKey, model.SessionUser{Username: "nobody"})

	l := NewLogoutLogic(ctx, newTestSvcCtx())
	out, err := l.Logout("not-exist-token")
	if err != nil {
		t.Fatalf("期望对不存在的会话登出不报错，却报错: %v", err)
	}
	if out.Username != "nobody" {
		t.Fatalf("期望登出用户名是 nobody，实际是 %s", out.Username)
	}
}
