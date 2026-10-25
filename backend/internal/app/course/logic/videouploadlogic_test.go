package logic

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// makeFakeVideoBytes 构造带 MP4 魔数（ftyp）的假视频字节，后面填充任意内容。
func makeFakeVideoBytes(extra int) []byte {
	head := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	return append(head, bytes.Repeat([]byte{0x00}, extra)...)
}

// 测试用例 1：一次上传多个合法视频 → 全部成功，返回视频列表，课程已追加
func TestVideoUploadLogic_Success(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, err := course.Store.AddCourse("MinIO 视频测试课程", "", "", "tester")
	if err != nil {
		t.Fatalf("准备课程失败: %v", err)
	}

	resp, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: c.Id,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 1024, Reader: bytes.NewReader(makeFakeVideoBytes(1024))},
			{FileName: "lesson2.mp4", Size: 2048, Reader: bytes.NewReader(makeFakeVideoBytes(2048))},
		},
	})
	if err != nil {
		t.Fatalf("期望视频上传成功，却报错: %v", err)
	}
	if len(resp.Videos) != 2 {
		t.Fatalf("期望返回 2 个视频，实际 %d 个", len(resp.Videos))
	}

	// 视频 id 从 1 开始自增
	if resp.Videos[0].Id != 1 || resp.Videos[1].Id != 2 {
		t.Fatalf("期望视频 id 为 1、2，实际 %d、%d", resp.Videos[0].Id, resp.Videos[1].Id)
	}

	// URL 非空且以 .mp4 结尾（对象名 = course/{id}/videos/{uuid}.mp4）
	for _, v := range resp.Videos {
		if v.Url == "" || !strings.HasSuffix(v.Url, ".mp4") {
			t.Fatalf("视频 URL 非法: %s", v.Url)
		}
		if v.UploadedAt == "" {
			t.Fatal("期望记录上传时间，实际为空")
		}
	}

	// 课程存储里也应追加了 2 条视频记录
	got, _ := course.Store.GetCourse(c.Id)
	if len(got.Videos) != 2 {
		t.Fatalf("期望课程存储有 2 条视频，实际 %d 条", len(got.Videos))
	}

	// 2 个对象都上传成功，且无回滚
	fake := ctx.Oss.(*fakeOss)
	if len(fake.objects) != 2 {
		t.Fatalf("期望上传 2 个对象，实际 %d 个", len(fake.objects))
	}
	if len(fake.removed) != 0 {
		t.Fatalf("成功场景不应有回滚删除，实际: %v", fake.removed)
	}
}

// 测试用例 2：课程不存在 → 上传失败
func TestVideoUploadLogic_CourseNotFound(t *testing.T) {
	l := NewVideoUploadLogic(context.Background(), newUploadTestSvcCtx())

	_, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: 99999,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 100, Reader: bytes.NewReader(makeFakeVideoBytes(100))},
		},
	})
	if err == nil || err.Error() != "课程不存在" {
		t.Fatalf("期望课程不存在报错，实际: %v", err)
	}
}

// 测试用例 3：一个文件都不传 → 上传失败
func TestVideoUploadLogic_EmptyFiles(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 视频空列表", "", "", "tester")
	_, err := l.UploadVideos(&course.UploadVideosRequest{CourseId: c.Id, Files: nil})
	if err == nil {
		t.Fatal("期望至少上传一个文件时报错，却没有报错")
	}
}

// 测试用例 4：第二个文件扩展名非法 → 整体失败，第一个已上传对象被回滚
func TestVideoUploadLogic_RollbackOnBadFile(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	fake := ctx.Oss.(*fakeOss)
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 视频回滚", "", "", "tester")
	_, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: c.Id,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 100, Reader: bytes.NewReader(makeFakeVideoBytes(100))},
			{FileName: "evil.exe", Size: 100, Reader: bytes.NewReader([]byte("MZ..."))},
		},
	})
	if err == nil {
		t.Fatal("期望第二个文件扩展名不支持时报错，却没有报错")
	}

	// 第一个已上传的对象应被回滚
	if len(fake.removed) != 1 {
		t.Fatalf("期望回滚 1 个已上传对象，实际 %d 个: %v", len(fake.removed), fake.removed)
	}
	// 课程视频列表不应被写脏
	got, _ := course.Store.GetCourse(c.Id)
	if len(got.Videos) != 0 {
		t.Fatalf("上传失败后课程不应有视频记录，实际 %d 条", len(got.Videos))
	}
}

// 测试用例 5：内容魔数非法（扩展名是 mp4，内容不是）→ 上传失败
func TestVideoUploadLogic_BadMagic(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 视频坏魔数", "", "", "tester")
	_, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: c.Id,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 32, Reader: bytes.NewReader([]byte("not a real video at all!!"))},
		},
	})
	if err == nil {
		t.Fatal("期望非视频内容被拒绝，却没有报错")
	}
}

// 测试用例 6：视频超过大小上限 → 上传失败
func TestVideoUploadLogic_TooLarge(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	ctx.Config.Upload.MaxVideoBytes = 100 // 把上限调小，便于构造超限数据
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 视频过大", "", "", "tester")
	_, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: c.Id,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 200, Reader: bytes.NewReader(makeFakeVideoBytes(200))},
		},
	})
	if err == nil {
		t.Fatal("期望超限视频被拒绝，却没有报错")
	}
}

// 测试用例 7：空视频文件（大小 0）→ 上传失败
func TestVideoUploadLogic_EmptyFile(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewVideoUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 视频空文件", "", "", "tester")
	_, err := l.UploadVideos(&course.UploadVideosRequest{
		CourseId: c.Id,
		Files: []*course.UploadFile{
			{FileName: "lesson1.mp4", Size: 0, Reader: bytes.NewReader(nil)},
		},
	})
	if err == nil {
		t.Fatal("期望空文件被拒绝，却没有报错")
	}
}
