package logic

import (
	"context"
	"testing"

	"learningplatform/internal/app/user"
	"learningplatform/internal/model"
)

// 测试用例 1：context 里有会话信息 → 返回当前登录用户
func TestInfoLogic_Success(t *testing.T) {
	// 模拟鉴权中间件把 Redis 会话解析后放入 context
	ctx := context.WithValue(context.Background(), user.AuthSessionKey,
		model.SessionUser{Username: "admin", Nickname: "管理员"})

	l := NewInfoLogic(ctx, newTestSvcCtx())
	resp, err := l.Info()
	if err != nil {
		t.Fatalf("期望获取用户信息成功，却报错: %v", err)
	}
	if resp.Username != "admin" {
		t.Fatalf("期望用户名是 admin，实际是 %s", resp.Username)
	}
	if resp.Nickname != "管理员" {
		t.Fatalf("期望昵称是 管理员，实际是 %s", resp.Nickname)
	}
}

// 测试用例 2：context 里没有会话 → 报错（正常情况下中间件会先拦截返回 401）
func TestInfoLogic_NoSession(t *testing.T) {
	l := NewInfoLogic(context.Background(), newTestSvcCtx())

	_, err := l.Info()
	if err == nil {
		t.Fatal("期望无会话时报错，却没有报错")
	}
}
