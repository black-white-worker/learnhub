package course

import (
	"learningplatform/internal/svc"
	"strings"
)

// ===== 课程资源（PPT / Word / PDF）上传的通用校验 =====
// 独立成文件，避免和封面 / 视频的校验逻辑互相干扰。

// defaultMaxResourceBytes 单个文档资源大小上限（50MB）。
// 文档比视频小得多，默认给 50MB 足够装下 PPT / Word / PDF。
const defaultMaxResourceBytes = 50 << 20

// EffectiveMaxResourceBytes 资源大小上限（教学版直接用默认值，不读配置）。
func EffectiveMaxResourceBytes(svcCtx *svc.ServiceContext) int64 {
	_ = svcCtx
	return defaultMaxResourceBytes
}

// AllowedResourceExt 允许上传的文档资源扩展名。
// 覆盖教学场景最常见的讲义格式：PDF 课件、PPT 演示、Word 文档。
var AllowedResourceExt = map[string]struct{}{
	".pdf":  {},
	".ppt":  {},
	".pptx": {},
	".doc":  {},
	".docx": {},
}

// IsAllowedResource 判断扩展名是否在文档资源白名单内。
func IsAllowedResource(ext string) bool {
	_, ok := AllowedResourceExt[strings.ToLower(ext)]
	return ok
}

// ResourceTypeByExt 把扩展名映射成简短类型名（pdf / ppt / doc），供前端图标展示。
func ResourceTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "pdf"
	case ".ppt", ".pptx":
		return "ppt"
	case ".doc", ".docx":
		return "doc"
	default:
		return "file"
	}
}

// ResourceContentTypeByExt 返回 MIME 类型（写进 MinIO 对象的 Content-Type），
// 让浏览器点开 PDF 时能直接预览而不是直接下载。
func ResourceContentTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".ppt":
		return "application/vnd.ms-powerpoint"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}
