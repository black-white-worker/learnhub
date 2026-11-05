package course

// 缩略图测试要用到的图片生成器。
// 原本只定义在 course/logic 包（coveruploadlogic_test.go），
// thumbnail_test.go 在 course 包里引用不到，这里补一份（行为一致）。

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

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
