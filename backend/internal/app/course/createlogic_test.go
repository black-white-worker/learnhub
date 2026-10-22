package course

import (
	"context"
	"testing"

	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// 测试辅助函数：构造一个 ServiceContext。
// 注意：不能走 svc.NewServiceContext —— 它内部会 redis.MustNewRedis 真连 Redis，
// 零配置下 host 为空直接 panic（合并 Redis 分支后引入的回归），
// 这里手动组装，只留测试需要的字段，不依赖外部 MySQL / Redis 服务。
func newTestSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			Upload: config.UploadConf{
				MaxCoverBytes: 5 << 20,   // 5MB
				MaxVideoBytes: 200 << 20, // 200MB
			},
			Minio: config.MinioConf{Bucket: "learningplatform"},
		},
		Oss: newFakeOss(),
	}
}

// 测试用例 1：创建一门全新课程 → 成功，返回 id 和名称
func TestCreateLogic_Success(t *testing.T) {
	l := NewCreateLogic(context.Background(), newTestSvcCtx())

	resp, err := l.Create(&CreateRequest{Name: "计算机网络", Intro: "TCP/IP 协议栈与网络编程", Teacher: "王老师"})
	if err != nil {
		t.Fatalf("期望创建成功，却报错: %v", err)
	}
	if resp.Name != "计算机网络" {
		t.Fatalf("期望返回名称 计算机网络，实际是 %s", resp.Name)
	}
	if resp.Id <= 0 {
		t.Fatalf("期望返回合法的课程 id，实际是 %d", resp.Id)
	}
}

// 测试用例 2：课程名与预置课程重复 → 创建失败
func TestCreateLogic_DuplicateCourse(t *testing.T) {
	l := NewCreateLogic(context.Background(), newTestSvcCtx())

	_, err := l.Create(&CreateRequest{Name: "Go 语言基础"})
	if err == nil {
		t.Fatal("期望课程名称已存在时报错，却没有报错")
	}
}

// 测试用例 3：课程名称为空 → 创建失败
func TestCreateLogic_EmptyParam(t *testing.T) {
	l := NewCreateLogic(context.Background(), newTestSvcCtx())

	_, err := l.Create(&CreateRequest{Name: ""})
	if err == nil {
		t.Fatal("期望课程名称为空时报错，却没有报错")
	}
}
