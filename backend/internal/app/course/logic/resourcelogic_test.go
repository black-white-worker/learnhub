package logic

import (
	"context"
	"testing"

	"learningplatform/internal/app/course"
)

// 「课程资源」分支：上传 / 列表 / 删除文档资源的单元测试。
// 写操作都针对"新建出来的课程"，不碰预置的 id=1/2，避免污染同包其他测试。

func mustAddTempCourseForResource(t *testing.T, name string) int {
	t.Helper()
	c, err := course.Store.AddCourse(name, "资源测试", "老师", "tester")
	if err != nil {
		t.Fatalf("建课失败: %v", err)
	}
	return c.Id
}

func TestResourceUpload_Success(t *testing.T) {
	id := mustAddTempCourseForResource(t, "资源测试课-上传")
	l := NewResourceUploadLogic(context.Background(), newTestSvcCtx())

	resp, err := l.UploadResource(&course.UploadResourceRequest{
		CourseId:  id,
		FileName:  "第一章讲义.pdf",
		FileBytes: []byte("%PDF-1.4 fake"),
	})
	if err != nil {
		t.Fatalf("期望上传成功，却报错: %v", err)
	}
	if resp.Resource.Name != "第一章讲义.pdf" || resp.Resource.Type != "pdf" {
		t.Fatalf("返回资源元信息不对: %+v", resp.Resource)
	}

	// 列表里应能查到
	list, _ := course.Store.ListResources(id)
	if len(list) != 1 {
		t.Fatalf("期望资源列表有 1 条，实际 %d", len(list))
	}
}

func TestResourceUpload_BadExt(t *testing.T) {
	id := mustAddTempCourseForResource(t, "资源测试课-后缀")
	l := NewResourceUploadLogic(context.Background(), newTestSvcCtx())

	_, err := l.UploadResource(&course.UploadResourceRequest{
		CourseId:  id,
		FileName:  "病毒.exe",
		FileBytes: []byte("MZ fake"),
	})
	if err == nil {
		t.Fatal("期望 exe 被拒绝，却没报错")
	}
}

func TestResourceUpload_Empty(t *testing.T) {
	id := mustAddTempCourseForResource(t, "资源测试课-空")
	l := NewResourceUploadLogic(context.Background(), newTestSvcCtx())

	_, err := l.UploadResource(&course.UploadResourceRequest{
		CourseId:  id,
		FileName:  "empty.pdf",
		FileBytes: nil,
	})
	if err == nil {
		t.Fatal("期望空文件被拒绝，却没报错")
	}
}

func TestResourceList_EmptyReturnsSlice(t *testing.T) {
	id := mustAddTempCourseForResource(t, "资源测试课-空列表")
	l := NewResourceListLogic(context.Background(), newTestSvcCtx())

	resp, err := l.List(&course.ListResourcesRequest{CourseId: id})
	if err != nil {
		t.Fatalf("期望列表成功，却报错: %v", err)
	}
	if resp.Resources == nil || len(resp.Resources) != 0 {
		t.Fatal("期望空列表返回空切片")
	}
}

func TestResourceDelete_Success(t *testing.T) {
	id := mustAddTempCourseForResource(t, "资源测试课-删除")
	up := NewResourceUploadLogic(context.Background(), newTestSvcCtx())
	resp, err := up.UploadResource(&course.UploadResourceRequest{
		CourseId:  id,
		FileName:  "a.docx",
		FileBytes: []byte("fake docx"),
	})
	if err != nil {
		t.Fatalf("前置上传失败: %v", err)
	}

	del := NewResourceDeleteLogic(context.Background(), newTestSvcCtx())
	if _, err := del.Delete(&course.DeleteResourceRequest{CourseId: id, ResourceId: resp.Resource.Id}); err != nil {
		t.Fatalf("期望删除成功，却报错: %v", err)
	}
	left, _ := course.Store.ListResources(id)
	if len(left) != 0 {
		t.Fatal("期望删除后资源列表为空")
	}
}

func TestResourceDelete_NotFound(t *testing.T) {
	l := NewResourceDeleteLogic(context.Background(), newTestSvcCtx())
	_, err := l.Delete(&course.DeleteResourceRequest{CourseId: 999999, ResourceId: 1})
	if err == nil {
		t.Fatal("期望删除不存在资源时报错，却没报错")
	}
}
