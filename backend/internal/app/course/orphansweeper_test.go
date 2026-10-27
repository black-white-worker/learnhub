package course

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ===== 孤儿对象清理测试 =====
// 用 fakeOss 直接造对象，不依赖真实 MinIO；store 是包级内存存储，直接造课程。

// 用例 1：未被引用的旧对象会被清理；被课程引用的对象保留
func TestSweepOrphans_RemovesUnreferenced(t *testing.T) {
	ctx := newTestSvcCtx()
	oss := ctx.Oss.(*fakeOss)

	// 1. 上传一个"孤儿"视频对象，并把时间拨回 30 分钟前（超过安全窗口 10 分钟）
	orphanName := "course/999/videos/orphan.mp4"
	if _, err := oss.PutObject(context.Background(), orphanName, strings.NewReader("x"), 1, "video/mp4"); err != nil {
		t.Fatalf("造孤儿对象失败: %v", err)
	}
	oss.backdate(orphanName, 30*time.Minute)

	// 2. 新建一门课程，并让它引用一个封面对象（被引用 → 应保留）
	c, err := Store.AddCourse("孤儿清理测试课", "", "", "tester")
	if err != nil {
		t.Fatalf("造课程失败: %v", err)
	}
	coverName := "course/" + strconv.Itoa(c.Id) + "/cover/original.png"
	coverURL := "http://fake-minio:9000/learningplatform/" + coverName
	if _, err := oss.PutObject(context.Background(), coverName, strings.NewReader("x"), 1, "image/png"); err != nil {
		t.Fatalf("造封面对象失败: %v", err)
	}
	if err := Store.SetCover(c.Id, coverURL, ""); err != nil {
		t.Fatalf("写课程封面引用失败: %v", err)
	}

	// 3. 执行清理
	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("清理报错: %v", err)
	}

	// 4. 断言：孤儿被删、被引用的封面保留
	if _, ok := oss.objects[orphanName]; ok {
		t.Fatalf("期望孤儿对象 %s 被清理，却还在", orphanName)
	}
	if _, ok := oss.objects[coverName]; !ok {
		t.Fatalf("期望被引用的封面 %s 保留，却被清理了", coverName)
	}
}

// 用例 2：太新的未引用对象不清理（安全窗口保护，防止误删"上传成功但引用未写回"的对象）
func TestSweepOrphans_KeepsYoungUnreferenced(t *testing.T) {
	ctx := newTestSvcCtx()
	oss := ctx.Oss.(*fakeOss)

	// 刚上传、未引用的对象（处于 10 分钟安全窗口内）
	youngName := "course/998/videos/young.mp4"
	if _, err := oss.PutObject(context.Background(), youngName, strings.NewReader("x"), 1, "video/mp4"); err != nil {
		t.Fatalf("造对象失败: %v", err)
	}

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("清理报错: %v", err)
	}

	if _, ok := oss.objects[youngName]; !ok {
		t.Fatalf("期望新对象 %s 在安全窗口内保留，却被清理了", youngName)
	}
}

// 用例 3：讲义（resources）必须被保留。
//
// 这是回归测试：历史实现只收集封面/视频引用，讲义上传 10 分钟后会被
// 当成孤儿对象删掉，用户看到的是"文件突然 404"。
func TestSweepOrphans_KeepsCourseResources(t *testing.T) {
	ctx := newTestSvcCtx()
	oss := ctx.Oss.(*fakeOss)

	c, err := Store.AddCourse("讲义保留测试课", "", "", "tester")
	if err != nil {
		t.Fatalf("造课程失败: %v", err)
	}
	objectName := "course/" + strconv.Itoa(c.Id) + "/resources/lecture.pdf"
	url := "http://fake-minio:9000/learningplatform/" + objectName
	if _, err := oss.PutObject(context.Background(), objectName, strings.NewReader("pdf"), 3, "application/pdf"); err != nil {
		t.Fatalf("造讲义对象失败: %v", err)
	}
	oss.backdate(objectName, 30*time.Minute) // 超过安全窗口，确保"可被清理"

	if err := Store.AddResource(c.Id, &Resource{
		Name: "lecture.pdf", Url: url, ObjectName: objectName, Size: 3, Type: "pdf",
	}); err != nil {
		t.Fatalf("写讲义引用失败: %v", err)
	}

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("清理报错: %v", err)
	}

	if _, ok := oss.objects[objectName]; !ok {
		t.Fatalf("讲义 %s 被误删了（引用集合漏了 resources）", objectName)
	}
}

// 用例 4：白名单之外的目录不清理（宁可留垃圾，不可误删将来新增的对象类型）。
func TestSweepOrphans_IgnoresUnknownPrefix(t *testing.T) {
	ctx := newTestSvcCtx()
	oss := ctx.Oss.(*fakeOss)

	unknown := "course/997/exports/backup.zip"
	if _, err := oss.PutObject(context.Background(), unknown, strings.NewReader("zip"), 3, "application/zip"); err != nil {
		t.Fatalf("造对象失败: %v", err)
	}
	oss.backdate(unknown, 24*time.Hour)

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("清理报错: %v", err)
	}
	if _, ok := oss.objects[unknown]; !ok {
		t.Fatalf("白名单外的对象 %s 不应被删除", unknown)
	}
}
