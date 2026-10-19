package course

import (
	"context"
	"testing"

	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// 测试辅助函数：构造一个 ServiceContext（测试只用它撑起 ListLogic 的结构）。
func newTestSvcCtx() *svc.ServiceContext {
	return svc.NewServiceContext(config.Config{})
}

// 测试用例 1：列表非空（存储里预置了两条课程）
func TestListLogic_NotEmpty(t *testing.T) {
	l := NewListLogic(context.Background(), newTestSvcCtx())

	resp, err := l.List()
	if err != nil {
		t.Fatalf("期望列表接口成功，却报错: %v", err)
	}
	if len(resp.Courses) == 0 {
		t.Fatal("期望课程列表非空，实际为空")
	}
}

// 测试用例 2：列表包含预置的"Go 语言基础"
func TestListLogic_ContainsPresetCourse(t *testing.T) {
	l := NewListLogic(context.Background(), newTestSvcCtx())

	resp, err := l.List()
	if err != nil {
		t.Fatalf("期望列表接口成功，却报错: %v", err)
	}
	found := false
	for _, c := range resp.Courses {
		if c.Name == "Go 语言基础" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("期望列表包含预置课程 Go 语言基础，实际没有")
	}
}
