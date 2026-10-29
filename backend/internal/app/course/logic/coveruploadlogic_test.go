package logic

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"learningplatform/internal/app/course"
	"testing"
)

// makeTestImageBytes 生成一张 PNG 测试图（用于封面上传 / 缩略图测试）。
func makeTestImageBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("生成测试图片失败: %v", err)
	}
	return buf.Bytes()
}

// 测试用例 1：上传合法封面 → 成功，返回原图和缩略图 URL，课程封面已更新
func TestCoverUploadLogic_Success(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewCoverUploadLogic(context.Background(), ctx)

	// 先创建一门课程（用唯一名称，避免与其它测试互相影响）
	c, err := course.Store.AddCourse("MinIO 封面测试课程", "测试封面上传", "测试老师", "tester")
	if err != nil {
		t.Fatalf("准备课程失败: %v", err)
	}

	resp, err := l.UploadCover(&course.UploadCoverRequest{
		CourseId:  c.Id,
		FileName:  "cover.png",
		FileBytes: makeTestImageBytes(t, 800, 450),
	})
	if err != nil {
		t.Fatalf("期望封面上传成功，却报错: %v", err)
	}
	if resp.CoverUrl == "" || resp.CoverThumbUrl == "" {
		t.Fatal("期望返回原图和缩略图 URL，实际为空")
	}

	// 课程封面应被写回存储
	got, _ := course.Store.GetCourse(c.Id)
	if got.CoverUrl != resp.CoverUrl || got.CoverThumbUrl != resp.CoverThumbUrl {
		t.Fatalf("课程封面未正确更新: cover=%s thumb=%s", got.CoverUrl, got.CoverThumbUrl)
	}

	// 原图 + 缩略图两个对象都应已上传
	fake := ctx.Oss.(*fakeOss)
	if len(fake.objects) != 2 {
		t.Fatalf("期望上传 2 个对象，实际 %d 个: %v", len(fake.objects), fake.objects)
	}
	if len(fake.removed) != 0 {
		t.Fatalf("成功场景不应有回滚删除，实际: %v", fake.removed)
	}
}

// 测试用例 2：课程不存在 → 上传失败
func TestCoverUploadLogic_CourseNotFound(t *testing.T) {
	l := NewCoverUploadLogic(context.Background(), newUploadTestSvcCtx())

	_, err := l.UploadCover(&course.UploadCoverRequest{
		CourseId:  99999,
		FileName:  "cover.png",
		FileBytes: makeTestImageBytes(t, 100, 100),
	})
	if err == nil || err.Error() != "课程不存在" {
		t.Fatalf("期望课程不存在报错，实际: %v", err)
	}
}

// 测试用例 3：非图片内容（魔数不合法）→ 上传失败
func TestCoverUploadLogic_BadMagic(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	l := NewCoverUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 封面坏魔数", "", "", "tester")
	_, err := l.UploadCover(&course.UploadCoverRequest{
		CourseId:  c.Id,
		FileName:  "cover.png", // 扩展名是图片，但内容不是
		FileBytes: []byte("this is not an image file at all........"),
	})
	if err == nil {
		t.Fatal("期望非图片文件被拒绝，却没有报错")
	}

	fake := ctx.Oss.(*fakeOss)
	if len(fake.objects) != 0 {
		t.Fatalf("校验失败时不应上传任何对象，实际: %v", fake.objects)
	}
}

// 测试用例 4：封面超过大小上限 → 上传失败
func TestCoverUploadLogic_TooLarge(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	ctx.Config.Upload.MaxCoverBytes = 100 // 把上限调小，便于构造超限数据
	l := NewCoverUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 封面过大", "", "", "tester")
	big := bytes.Repeat([]byte{0xFF}, 101) // 101 字节，超过 100
	_, err := l.UploadCover(&course.UploadCoverRequest{
		CourseId:  c.Id,
		FileName:  "cover.jpg",
		FileBytes: big,
	})
	if err == nil {
		t.Fatal("期望超限封面被拒绝，却没有报错")
	}
}

// 测试用例 5：缩略图上传失败 → 已上传的原图被回滚删除
func TestCoverUploadLogic_RollbackThumbFail(t *testing.T) {
	ctx := newUploadTestSvcCtx()
	fake := ctx.Oss.(*fakeOss)
	fake.failOn = 2 // 第 2 次 PutObject（缩略图）失败
	l := NewCoverUploadLogic(context.Background(), ctx)

	c, _ := course.Store.AddCourse("MinIO 封面回滚", "", "", "tester")
	_, err := l.UploadCover(&course.UploadCoverRequest{
		CourseId:  c.Id,
		FileName:  "cover.png",
		FileBytes: makeTestImageBytes(t, 100, 100),
	})
	if err == nil {
		t.Fatal("期望缩略图上传失败时报错，却没有报错")
	}

	// 原图应被回滚删除，且课程封面没有被写脏
	if len(fake.removed) != 1 {
		t.Fatalf("期望回滚删除 1 个对象，实际 %d 个: %v", len(fake.removed), fake.removed)
	}
	got, _ := course.Store.GetCourse(c.Id)
	if got.CoverUrl != "" || got.CoverThumbUrl != "" {
		t.Fatalf("上传失败后课程封面不应被更新，实际 cover=%s thumb=%s", got.CoverUrl, got.CoverThumbUrl)
	}
}
