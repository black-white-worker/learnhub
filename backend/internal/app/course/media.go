package course

// 媒体 URL 的签名（私有桶支持）。
//
// 现状：compose 里的 minio-init 给 course/ 前缀开了 anonymous download，
// 拿到 URL 的人可以无限期直接下载/盗链，视频也能被别的站引用。
//
// 目标形态：私有桶 + 读接口按登录态返回**有时效**的预签名 URL。
// 由于这会改变存量部署的行为（关掉匿名读后旧直链立刻失效），
// 这里做成开关：Minio.Private=true 时签名，默认 false 保持兼容。
//
// 实现上用"可选能力"类型断言，不污染 oss.ObjectStorage 接口
// （否则所有测试用的 fake 实现都要跟着加方法）。

import (
	"context"
	"time"

	"learningplatform/internal/svc"
)

// mediaPresignTTL 预签名 URL 有效期。
// 30 分钟足够看完一节课，又不至于让链接被长期传播。
const mediaPresignTTL = 30 * time.Minute

// getPresigner 对象存储的"可选能力"：支持预签名下载。
type getPresigner interface {
	PresignGetObject(ctx context.Context, objectName string, expiry time.Duration) (string, error)
}

// MediaURL 返回可访问的媒体 URL。
//
//   - svcCtx.Config.Minio.Private = false：原样返回库里存的公开 URL（当前默认行为）；
//   - Private = true：用 objectName 签一个 30 分钟的下载 URL；签名失败时退回原 URL，
//     保证"配置了私有桶但 MinIO 暂时不可用"时页面不至于整块挂掉（安全性由桶策略兜底）。
func MediaURL(svcCtx *svc.ServiceContext, objectName, fallback string) string {
	if svcCtx == nil || !svcCtx.Config.Minio.Private || objectName == "" {
		return fallback
	}
	p, ok := svcCtx.Oss.(getPresigner)
	if !ok {
		return fallback
	}
	signed, err := p.PresignGetObject(context.Background(), objectName, mediaPresignTTL)
	if err != nil {
		return fallback
	}
	return signed
}
