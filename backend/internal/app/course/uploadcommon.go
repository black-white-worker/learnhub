package course

import (
	"bytes"
	"strings"

	"learningplatform/internal/svc"
)

// ===== 上传通用：大小默认值、扩展名白名单、文件魔数校验 =====

// 配置未设置时的默认大小上限（字节）
const (
	defaultMaxCoverBytes = 5 << 20   // 封面 5MB
	defaultMaxVideoBytes = 500 << 20 // 单个视频 500MB
)

// EffectiveMaxCoverBytes 取配置的封面上限；配置为 0（未设置）时用默认值。
func EffectiveMaxCoverBytes(svcCtx *svc.ServiceContext) int64 {
	if svcCtx.Config.Upload.MaxCoverBytes > 0 {
		return svcCtx.Config.Upload.MaxCoverBytes
	}
	return defaultMaxCoverBytes
}

// EffectiveMaxVideoBytes 取配置的单个视频上限；配置为 0（未设置）时用默认值。
func EffectiveMaxVideoBytes(svcCtx *svc.ServiceContext) int64 {
	if svcCtx.Config.Upload.MaxVideoBytes > 0 {
		return svcCtx.Config.Upload.MaxVideoBytes
	}
	return defaultMaxVideoBytes
}

// 允许上传的图片扩展名（与 CheckImageMagic 支持的格式保持一致）
var AllowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
}

// 允许上传的视频扩展名
var AllowedVideoExt = map[string]struct{}{
	".mp4":  {},
	".mov":  {},
	".webm": {},
	".avi":  {},
	".mkv":  {},
}

// IsAllowedImage 判断扩展名是否在图片白名单内
func IsAllowedImage(ext string) bool {
	_, ok := AllowedImageExt[strings.ToLower(ext)]
	return ok
}

// IsAllowedVideo 判断扩展名是否在视频白名单内
func IsAllowedVideo(ext string) bool {
	_, ok := AllowedVideoExt[strings.ToLower(ext)]
	return ok
}

// CheckImageMagic 校验图片文件魔数（文件头签名），
// 防止"改扩展名伪装图片"或上传非图片文件。支持 JPEG / PNG / GIF。
func CheckImageMagic(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	// JPEG: FF D8 FF
	if b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF {
		return true
	}
	// PNG: 89 50 4E 47 0D 0A 1A 0A
	if bytes.Equal(b[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		return true
	}
	// GIF: "GIF8"
	if string(b[:4]) == "GIF8" {
		return true
	}
	return false
}

// CheckVideoMagic 校验视频文件魔数（文件头签名）。
// 支持 MP4/MOV（ftyp）、WebM/MKV（EBML）、AVI（RIFF...AVI）。
func CheckVideoMagic(b []byte) bool {
	if len(b) < 12 {
		return false
	}
	// MP4/MOV/3GP: 偏移 4 处是 "ftyp"
	if string(b[4:8]) == "ftyp" {
		return true
	}
	// WebM/MKV: EBML 头 1A 45 DF A3
	if bytes.Equal(b[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}) {
		return true
	}
	// AVI: "RIFF" + 大小 + "AVI "
	if string(b[:4]) == "RIFF" && string(b[8:12]) == "AVI " {
		return true
	}
	return false
}

// ContentTypeByExt 根据扩展名返回 MIME 类型（写进 MinIO 对象的 Content-Type）。
func ContentTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".webm":
		return "video/webm"
	case ".avi":
		return "video/x-msvideo"
	case ".mkv":
		return "video/x-matroska"
	default:
		return "application/octet-stream"
	}
}
