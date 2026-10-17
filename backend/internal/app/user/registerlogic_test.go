package user

import (
	"context"
	"testing"
)

// 测试用例 1：注册一个全新用户 → 成功，返回用户名
func TestRegisterLogic_Success(t *testing.T) {
	l := NewRegisterLogic(context.Background(), newTestSvcCtx())

	resp, err := l.Register(&RegisterRequest{Username: "zhangsan", Password: "123456"})
	if err != nil {
		t.Fatalf("期望注册成功，却报错: %v", err)
	}
	if resp.Username != "zhangsan" {
		t.Fatalf("期望返回用户名 zhangsan，实际是 %s", resp.Username)
	}
}

// 测试用例 2：用户名已存在（admin 是预置账号）→ 注册失败
func TestRegisterLogic_DuplicateUser(t *testing.T) {
	l := NewRegisterLogic(context.Background(), newTestSvcCtx())

	_, err := l.Register(&RegisterRequest{Username: "admin", Password: "123456"})
	if err == nil {
		t.Fatal("期望用户名已存在时报错，却没有报错")
	}
}

// 测试用例 3：用户名或密码为空 → 注册失败
func TestRegisterLogic_EmptyParam(t *testing.T) {
	l := NewRegisterLogic(context.Background(), newTestSvcCtx())

	_, err := l.Register(&RegisterRequest{Username: "", Password: ""})
	if err == nil {
		t.Fatal("期望参数为空时报错，却没有报错")
	}
}
