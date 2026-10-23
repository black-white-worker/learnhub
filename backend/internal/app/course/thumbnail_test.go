package course

import (
	"bytes"
	"image"
	_ "image/jpeg"
	"testing"
)

// 验证缩略图生成：输出必须是 320x180 的 JPEG。
func TestGenerateThumbnail(t *testing.T) {
	src := makeTestImageBytes(t, 1600, 900) // 横图：宽高比 16:9
	thumb, err := generateThumbnail(src, thumbWidth, thumbHeight)
	if err != nil {
		t.Fatalf("生成缩略图失败: %v", err)
	}

	img, format, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("缩略图无法解码: %v", err)
	}
	if format != "jpeg" {
		t.Fatalf("期望 JPEG 缩略图，实际 %s", format)
	}
	b := img.Bounds()
	if b.Dx() != thumbWidth || b.Dy() != thumbHeight {
		t.Fatalf("期望 %dx%d，实际 %dx%d", thumbWidth, thumbHeight, b.Dx(), b.Dy())
	}
}

// 竖图（比例与 16:9 不同）也应被居中裁剪成 320x180。
func TestGenerateThumbnail_PortraitImage(t *testing.T) {
	src := makeTestImageBytes(t, 450, 800) // 竖图
	thumb, err := generateThumbnail(src, thumbWidth, thumbHeight)
	if err != nil {
		t.Fatalf("生成缩略图失败: %v", err)
	}
	img, _, err := image.Decode(bytes.NewReader(thumb))
	if err != nil {
		t.Fatalf("缩略图无法解码: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != thumbWidth || b.Dy() != thumbHeight {
		t.Fatalf("期望 %dx%d，实际 %dx%d", thumbWidth, thumbHeight, b.Dx(), b.Dy())
	}
}

// 非图片内容 → 生成缩略图失败。
func TestGenerateThumbnail_BadImage(t *testing.T) {
	_, err := generateThumbnail([]byte("this is not an image........"), thumbWidth, thumbHeight)
	if err == nil {
		t.Fatal("期望非图片内容生成失败，却没有报错")
	}
}
