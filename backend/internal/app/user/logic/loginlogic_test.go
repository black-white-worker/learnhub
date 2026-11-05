package logic

import (
	"context"
	"os"
	"strings"
	"testing"

	"learningplatform/internal/app/user"
	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// 测试辅助函数：构造一个连接 MySQL + Redis 的 ServiceContext。
// 注意：接入数据库后这些变成集成测试，需要先启动 MySQL 和 Redis
// （推荐：在项目根目录执行 docker compose up -d mysql redis）。
// 连接信息一律从环境变量读取，不写死在仓库里：
//
//	TEST_MYSQL_DSN    形如 root:<密码>@tcp(127.0.0.1:3316)/learningplatform?charset=utf8mb4&parseTime=true&loc=Local
//	TEST_REDIS_PASS   Redis 密码（无密码时设为 TEST_REDIS_PASS=）
func newTestSvcCtx() *svc.ServiceContext {
	c := config.Config{}
	c.Mysql.DataSource = os.Getenv("TEST_MYSQL_DSN")
	// 注意：代码里直接构造 Config 时，Type 的 "node" 默认值不会自动生效，需显式指定
	c.Redis.Type = "node"
	c.Redis.Host = "127.0.0.1:6381"
	c.Redis.Pass = os.Getenv("TEST_REDIS_PASS")
	return svc.NewServiceContext(c)
}

// 依赖判定与整包跳过统一放在 main_test.go 的 TestMain 里（一个包只能有一个 TestMain），
// 这里不要再声明。

// 测试用例 1：用户名密码都正确 → 登录成功，返回 token（admin/123456 由初始化 SQL 预置）
func TestLoginLogic_Success(t *testing.T) {
	svcCtx := newTestSvcCtx()
	l := NewLoginLogic(context.Background(), svcCtx)

	resp, err := l.Login(&user.LoginRequest{Username: "admin", Password: "123456"})
	if err != nil {
		t.Fatalf("期望登录成功，却报错: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("期望返回 token，却是空的")
	}
	if resp.Username != "admin" {
		t.Fatalf("期望用户名是 admin，实际是 %s", resp.Username)
	}

	// 登录后 Redis 里应有会话 JSON（含用户名与昵称）
	val, err := svcCtx.Redis.Get(user.SessionKeyPrefix + resp.Token)
	if err != nil || val == "" {
		t.Fatal("期望登录后 Redis 里有会话，却没有")
	}
	if !strings.Contains(val, `"username":"admin"`) {
		t.Fatalf("期望会话 JSON 包含用户名 admin，实际是 %s", val)
	}
}

// 测试用例 2：密码错误 → 登录失败
func TestLoginLogic_WrongPassword(t *testing.T) {
	l := NewLoginLogic(context.Background(), newTestSvcCtx())

	_, err := l.Login(&user.LoginRequest{Username: "admin", Password: "wrong-password"})
	if err == nil {
		t.Fatal("期望密码错误时报错，却没有报错")
	}
}

// 测试用例 3：用户不存在 → 登录失败
func TestLoginLogic_UnknownUser(t *testing.T) {
	l := NewLoginLogic(context.Background(), newTestSvcCtx())

	_, err := l.Login(&user.LoginRequest{Username: "nobody", Password: "123456"})
	if err == nil {
		t.Fatal("期望用户不存在时报错，却没有报错")
	}
}
