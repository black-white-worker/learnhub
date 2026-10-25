package course

import (
	"bytes"
	"errors"

	"github.com/disintegration/imaging"
)

// 封面缩略图规格：320x180（课程卡片常用的 16:9 尺寸）
const (
	thumbWidth  = 320
	thumbHeight = 180
)

// generateThumbnail 把封面图等比缩放并居中裁剪成指定尺寸的缩略图，输出 JPEG 字节。
// 用了两步处理：
//  1. 等比缩放，让图片尽量占满目标矩形；
//  2. 居中裁剪，把超出目标尺寸的边缘裁掉，保证正好是 320x180。
//
// 好处：所有课程的封面缩略图尺寸一致，前端列表布局不会因为图片比例不同而错位。
func GenerateThumbnail(src []byte, width, height int) ([]byte, error) {
	// 1. 解码图片（支持 JPEG/PNG/GIF；非图片或损坏文件直接解码失败）
	img, err := imaging.Decode(bytes.NewReader(src), imaging.AutoOrientation(true))
	if err != nil {
		return nil, errors.New("封面图片无法解码，可能已损坏")
	}

	// 2. 等比缩放 + 居中裁剪到目标尺寸（Lanczos 是高质量重采样算法）
	thumb := imaging.Fill(img, width, height, imaging.Center, imaging.Lanczos)

	// 3. 编码为 JPEG（80% 质量，兼顾清晰度与体积）
	var buf bytes.Buffer
	if err := imaging.Encode(&buf, thumb, imaging.JPEG, imaging.JPEGQuality(80)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
