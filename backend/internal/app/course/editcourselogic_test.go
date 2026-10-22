package course

import (
	"context"
	"testing"
)

// 本文件覆盖「课程编辑」分支新增的三个接口：编辑、详情、删除。
// store 是包级单例，这里所有写操作都针对"新建出来的课程"，
// 不去改预置的 id=1/2，避免和同包其他测试互相污染。

// 辅助：新建一门课程并返回它的 id。
func mustAddTempCourse(t *testing.T, name string) int {
	t.Helper()
	c, err := store.AddCourse(name, "临时简介", "临时老师")
	if err != nil {
		t.Fatalf("临时建课 %s 失败: %v", name, err)
	}
	return c.Id
}

// 编辑：正常改名成功
func TestEditCourseLogic_Success(t *testing.T) {
	id := mustAddTempCourse(t, "测试编辑原始名")
	l := NewEditCourseLogic(context.Background(), newTestSvcCtx())

	resp, err := l.EditCourse(&EditCourseRequest{Id: id, Name: "概率论", Intro: "概率与统计", Teacher: "陈老师"})
	if err != nil {
		t.Fatalf("期望编辑成功，却报错: %v", err)
	}
	if resp.Name != "概率论" || resp.Teacher != "陈老师" {
		t.Fatalf("期望返回更新后的内容，实际 %+v", resp)
	}
}

// 编辑：课程名称为空 → 失败
func TestEditCourseLogic_EmptyName(t *testing.T) {
	id := mustAddTempCourse(t, "测试编辑空名")
	l := NewEditCourseLogic(context.Background(), newTestSvcCtx())

	_, err := l.EditCourse(&EditCourseRequest{Id: id, Name: ""})
	if err == nil {
		t.Fatal("期望课程名为空时报错，却没报错")
	}
}

// 编辑：改成和预置课程同名 → 重名失败
func TestEditCourseLogic_DuplicateName(t *testing.T) {
	id := mustAddTempCourse(t, "测试编辑重名")
	l := NewEditCourseLogic(context.Background(), newTestSvcCtx())

	// 预置课程里已有 "Go 语言基础"，改成它应被拦截
	_, err := l.EditCourse(&EditCourseRequest{Id: id, Name: "Go 语言基础"})
	if err == nil {
		t.Fatal("期望改成重名课程时报错，却没报错")
	}
}

// 编辑：编辑不存在的课程 → 失败
func TestEditCourseLogic_NotFound(t *testing.T) {
	l := NewEditCourseLogic(context.Background(), newTestSvcCtx())
	_, err := l.EditCourse(&EditCourseRequest{Id: 999999, Name: "不存在的课"})
	if err == nil {
		t.Fatal("期望编辑不存在的课程时报错，却没报错")
	}
}

// 详情：取预置课程 id=1 成功，且返回封面/视频字段
func TestCourseDetailLogic_Success(t *testing.T) {
	l := NewCourseDetailLogic(context.Background(), newTestSvcCtx())

	resp, err := l.Detail(&CourseDetailRequest{Id: 1})
	if err != nil {
		t.Fatalf("期望详情成功，却报错: %v", err)
	}
	if resp.Name != "Go 语言基础" {
		t.Fatalf("期望课程名 Go 语言基础，实际 %s", resp.Name)
	}
	if resp.Videos == nil {
		t.Fatal("期望空视频列表返回空切片而不是 nil")
	}
}

// 详情：课程不存在 → 失败
func TestCourseDetailLogic_NotFound(t *testing.T) {
	l := NewCourseDetailLogic(context.Background(), newTestSvcCtx())
	_, err := l.Detail(&CourseDetailRequest{Id: 999999})
	if err == nil {
		t.Fatal("期望详情不存在课程时报错，却没报错")
	}
}

// 删除：新建课程后删除成功，再查应不存在
func TestDeleteCourseLogic_Success(t *testing.T) {
	id := mustAddTempCourse(t, "待删除课程")
	l := NewDeleteCourseLogic(context.Background(), newTestSvcCtx())

	if _, err := l.Delete(&DeleteCourseRequest{Id: id}); err != nil {
		t.Fatalf("期望删除成功，却报错: %v", err)
	}
	if _, err := store.GetCourse(id); err == nil {
		t.Fatal("期望删除后再查不到该课程，实际还在")
	}
}

// 删除：删除不存在的课程 → 失败
func TestDeleteCourseLogic_NotFound(t *testing.T) {
	l := NewDeleteCourseLogic(context.Background(), newTestSvcCtx())
	_, err := l.Delete(&DeleteCourseRequest{Id: 999999})
	if err == nil {
		t.Fatal("期望删除不存在课程时报错，却没报错")
	}
}
