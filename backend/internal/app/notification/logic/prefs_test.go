package logic

// 通知偏好的单元测试。
//
// 两层：
//  1. 纯逻辑用例（TestPrefsLogic_UpdateEmptyChannel）—— 校验 channel 为空必须返回 400，
//     不依赖任何外部存储，随时可跑。
//  2. 集成用例（TestPrefsLogic_UpsertAndList）—— 真连 MySQL 验证 Upsert 幂等 + List 读回，
//     未配置 TEST_MYSQL_DSN 时整条跳过（与仓库其它集成测试约定一致）。

import (
	"context"
	"os"
	"testing"

	"learningplatform/internal/app/class"
	"learningplatform/internal/config"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// 仅用于纯逻辑用例：不连任何存储，Upsert 在空 channel 校验处就返回，不会触达 DB。
func newPureSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		NotificationPrefStore: model.NewNotificationPrefModel(sqlx.NewMysql("dummy")),
	}
}

func newTestSvcCtx() *svc.ServiceContext {
	c := config.Config{}
	c.Mysql.DataSource = os.Getenv("TEST_MYSQL_DSN")
	c.Redis.Type = "node"
	c.Redis.Host = "127.0.0.1:6381"
	c.Redis.Pass = os.Getenv("TEST_REDIS_PASS")
	return svc.NewServiceContext(c)
}

// 用例 1：channel 为空 → 400，且不应触达数据库。
func TestPrefsLogic_UpdateEmptyChannel(t *testing.T) {
	l := NewPrefsLogic(context.Background(), newPureSvcCtx())
	cu := &class.CtxUser{Id: 1, Username: "u", Role: "student"}

	err := l.Update(cu, "", 1)
	be, ok := err.(*class.BizError)
	if !ok {
		t.Fatalf("期望返回 *class.BizError，实际 %T: %v", err, err)
	}
	if be.Code != 400 {
		t.Fatalf("期望状态码 400，实际 %d", be.Code)
	}
}

// 用例 2（集成）：Upsert 幂等 + List 能读回最新值。
func TestPrefsLogic_UpsertAndList(t *testing.T) {
	if os.Getenv("TEST_MYSQL_DSN") == "" {
		t.Skip("未设置 TEST_MYSQL_DSN，跳过通知偏好集成测试")
	}
	l := NewPrefsLogic(context.Background(), newTestSvcCtx())
	// 用一个不太可能和真实数据冲突的固定 user_id
	cu := &class.CtxUser{Id: 424242, Username: "prefuser", Role: "student"}

	if err := l.Update(cu, "inapp", 1); err != nil {
		t.Fatalf("首次写入偏好失败: %v", err)
	}
	// 幂等覆盖：再写 enabled=0，应当更新而非新增
	if err := l.Update(cu, "inapp", 0); err != nil {
		t.Fatalf("更新偏好失败: %v", err)
	}

	resp, err := l.List(cu)
	if err != nil {
		t.Fatalf("列出偏好失败: %v", err)
	}
	found := false
	for _, p := range resp.Prefs {
		if p.Channel == "inapp" {
			found = true
			if p.Enabled != 0 {
				t.Fatalf("期望 inapp 被覆盖为 0，实际 %d", p.Enabled)
			}
		}
	}
	if !found {
		t.Fatal("List 未返回刚写入的 inapp 偏好")
	}
}
