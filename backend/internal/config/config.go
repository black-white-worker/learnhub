// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package config

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf

	// Mysql 数据库连接配置（DSN 由环境变量 MYSQL_DSN 注入）
	Mysql struct {
		DataSource string
	}

	// Redis 缓存配置（登录 token 等）
	Redis redis.RedisConf

	// Minio 对象存储配置（MinIO），用于保存课程封面 / 缩略图 / 视频文件
	Minio MinioConf

	// Upload 文件上传大小限制（字节）
	Upload UploadConf

	// AiService AI 子服务（ai-service）配置
	AiService AiServiceConf
}

// AiServiceConf ai-service（FastAPI）连接配置。
// 对应 etc/learningplatform-api.yaml 里的 AiService 段，值由环境变量注入。
//
// Url 与 InternalToken 任一为空 → /api/ai/* 直接回 503「AI 功能未启用」，
// 不注册转发逻辑，也不会因为连不上而 500。
type AiServiceConf struct {
	// Url ai-service 的根地址（容器网络内填 http://ai-service:8000）
	Url string
	// InternalToken 与 ai-service 共享的密钥，放请求头 X-Internal-Token。
	// ai-service 只信任带这个头的请求 —— 否则任何人都能绕过 Go
	// 直连 8000 端口伪造成别的用户。
	InternalToken string
}

// MinioConf MinIO 连接配置。
// 对应 etc/learningplatform-api.yaml 里的 Minio 段。
type MinioConf struct {
	Endpoint       string // MinIO 服务地址（后端连接用）。容器内填服务名 minio:9000，本机填 127.0.0.1:9020
	PublicEndpoint string // 对外访问地址（浏览器能打开的），用于拼接返回给前端的 URL；为空时退回 Endpoint
	AccessKey      string // 访问密钥
	SecretKey      string // 密钥
	UseSSL         bool   // 是否使用 HTTPS 访问 MinIO
	Bucket         string // 默认桶名（不存在会自动创建）
	Private        bool   // 是否私有桶：开启后读接口返回预签名 URL，不再依赖匿名读
}

// UploadConf 上传大小限制（字节）。
// 对应 etc/learningplatform-api.yaml 里的 Upload 段。
type UploadConf struct {
	MaxCoverBytes int64 // 封面原图大小上限（默认 5MB）
	MaxVideoBytes int64 // 单个视频大小上限（默认 800MB，也是全平台单文件硬上限）
	// DailyQuotaBytesPerUser 每个用户每日上传总量上限（字节），0 = 不限制。
	// 防的是"账号被盗后把对象存储灌满"：按用户 + 自然日计数（Redis，48h TTL）。
	DailyQuotaBytesPerUser int64
}
