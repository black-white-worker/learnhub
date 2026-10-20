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
}

// MinioConf MinIO 连接配置。
// 对应 etc/learningplatform-api.yaml 里的 Minio 段。
type MinioConf struct {
	Endpoint  string // MinIO 服务地址，如 127.0.0.1:9000
	AccessKey string // 访问密钥
	SecretKey string // 密钥
	UseSSL    bool   // 是否使用 HTTPS 访问 MinIO
	Bucket    string // 默认桶名（不存在会自动创建）
}

// UploadConf 上传大小限制（字节）。
// 对应 etc/learningplatform-api.yaml 里的 Upload 段。
type UploadConf struct {
	MaxCoverBytes int64 // 封面原图大小上限（默认 5MB）
	MaxVideoBytes int64 // 单个视频大小上限（默认 200MB）
}
