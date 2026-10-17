package user

import (
	"context"
	"testing"

	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// 测试辅助函数：构造一个带用户存储的 ServiceContext。
// 注意：这里传的是 config.Config 的零值，测试只用到 UserStore，不需要真正的服务器配置。
func newTestSvcCtx() *svc.ServiceContext {
	return svc.NewServiceContext(config.Config{})
}

// 测试用例 1：用户名密码都正确 → 登录成功，返回 token
func TestLoginLogic_Success(t *testing.T) {
	l := NewLoginLogic(context.Background(), newTestSvcCtx())

	resp, err := l.Login(&LoginRequest{Username: "admin", Password: "123456"})
	if err != nil {
		t.Fatalf("期望登录成功，却报错: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("期望返回 token，却是空的")
	}
	if resp.Username != "admin" {
		t.Fatalf("期望用户名是 admin，实际是 %s", resp.Username)
	}
}

// 测试用例 2：密码错误 → 登录失败
func TestLoginLogic_WrongPassword(t *testing.T) {
	l := NewLoginLogic(context.Background(), newTestSvcCtx())

	_, err := l.Login(&LoginRequest{Username: "admin", Password: "wrong-password"})
	if err == nil {
		t.Fatal("期望密码错误时报错，却没有报错")
	}
}

// 测试用例 3：用户不存在 → 登录失败
func TestLoginLogic_UnknownUser(t *testing.T) {
	l := NewLoginLogic(context.Background(), newTestSvcCtx())

	_, err := l.Login(&LoginRequest{Username: "nobody", Password: "123456"})
	if err == nil {
		t.Fatal("期望用户不存在时报错，却没有报错")
	}
}
