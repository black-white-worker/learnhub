// Package oss 对象存储（MinIO）封装。
//
// 设计说明：
// 业务代码（course 等模块）只依赖 ObjectStorage 接口，
// 生产环境注入 MinioStore，测试环境注入内存假实现（fakeOss），
// 这样上传逻辑可以不依赖真实 MinIO 服务做单元测试。
package oss

import (
	"context"
	"fmt"
	"io"
	"time"

	"learningplatform/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectInfo 对象存储中的一个对象信息（孤儿清理扫描用）。
type ObjectInfo struct {
	Name         string    // 桶内路径，如 course/1/videos/xxx.mp4
	LastModified time.Time // 对象最后修改时间（用于安全窗口判断）
}

// ObjectStorage 对象存储抽象接口。
type ObjectStorage interface {
	// PutObject 上传一个对象，返回可直接访问的 URL。
	// objectName 是桶内路径，如 course/1/cover/original.jpg。
	PutObject(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error)

	// RemoveObject 删除一个对象。
	// 用于上传中途失败时回滚"孤儿对象"，避免 MinIO 里残留半成品文件。
	RemoveObject(ctx context.Context, objectName string) error

	// Download 下载对象，返回可读流。
	// 供视频 faststart 转码时先把源视频拉到本地临时文件用（来自「课程观看」分支）。
	Download(ctx context.Context, objectName string) (io.ReadCloser, error)

	// ListObjects 列出指定前缀下的所有对象（递归），供孤儿对象清理扫描。
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)

	// PresignPutObject 生成一个前端可直接 PUT 的预签名 URL，
	// 后端不中转字节流，适合封面/资源/视频分片直传。
	PresignPutObject(ctx context.Context, objectName, contentType string, expiry time.Duration) (string, error)

	// ComposeObjects 把多个已存在的源对象按顺序合并成 dst 对象。
	// 分片上传场景：前端把每片 PUT 到 {objectName}.part1/.part2/...，
	// 后端调 ComposeObjects 合并成最终视频，再清理 parts。
	// 源对象除最后一片外必须 ≥ 5MB（MinIO/S3 限制）。
	ComposeObjects(ctx context.Context, dst string, srcs []string) (url string, err error)
}

// MultipartPart 保留给前端直传场景记录 ETag（本项目用 ComposeObject 方案，暂未使用 ETag）。
type MultipartPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

// MinioStore 基于 MinIO 的实现。
type MinioStore struct {
	client         *minio.Client // 实际连 MinIO 用（容器内 minio:9000）
	presignClient  *minio.Client // 仅用于生成预签名 URL（用 publicEndpoint 算签名）
	bucket         string        // 默认桶名
	endpoint       string        // 服务地址，用于拼 URL
	publicEndpoint string        // 对外访问地址（浏览器可访问），为空时退回 endpoint
	useSSL         bool
}

// NewMinioStore 创建 MinIO 客户端，并确保桶存在（不存在则自动创建）。
func NewMinioStore(cfg config.MinioConf) (*MinioStore, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO 客户端失败: %w", err)
	}

	m := &MinioStore{
		client:         client,
		bucket:         cfg.Bucket,
		endpoint:       cfg.Endpoint,
		publicEndpoint: cfg.PublicEndpoint,
		useSSL:         cfg.UseSSL,
	}
	// 没配 PublicEndpoint 时退回 Endpoint（本机直跑场景两者一致）
	if m.publicEndpoint == "" {
		m.publicEndpoint = m.endpoint
	}

	// presignClient 用 publicEndpoint 初始化：预签名 URL 的签名包含 host，
	// 如果用容器内 minio:9000 签出来再硬改 host，浏览器 PUT 时 SignatureDoesNotMatch。
	// 注意：minio-go 默认会发 GetBucketLocation 请求拿 region，容器内访问 127.0.0.1:9020
	// 会被拒，所以显式指定 Region 跳过这个请求（MinIO 单节点不关心 region 值）。
	presignClient, err := minio.New(m.publicEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: "us-east-1",
	})
	if err != nil {
		return nil, fmt.Errorf("创建 MinIO presign 客户端失败: %w", err)
	}
	m.presignClient = presignClient

	// 启动时确保桶存在，省去手动创建
	if err := m.ensureBucket(context.Background()); err != nil {
		return nil, fmt.Errorf("初始化 MinIO 桶失败: %w", err)
	}
	return m, nil
}

// ensureBucket 检查桶是否存在，不存在则创建。
func (m *MinioStore) ensureBucket(ctx context.Context) error {
	exists, err := m.client.BucketExists(ctx, m.bucket)
	if err != nil {
		return err
	}
	if !exists {
		return m.client.MakeBucket(ctx, m.bucket, minio.MakeBucketOptions{})
	}
	return nil
}

// PutObject 上传对象并返回访问 URL。
// 说明：这里直接返回 http://endpoint/bucket/object 形式的直链，
// 需要桶对外可读（教学版默认）；生产环境可改为预签名 URL 或自定义域名。
func (m *MinioStore) PutObject(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	_, err := m.client.PutObject(ctx, m.bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", fmt.Errorf("上传对象 %s 失败: %w", objectName, err)
	}

	scheme := "http"
	if m.useSSL {
		scheme = "https"
	}
	// 用 PublicEndpoint 拼 URL（浏览器能访问的地址），
	// 这样容器内连接用 minio:9000、浏览器访问用 127.0.0.1:9020 两不误。
	return fmt.Sprintf("%s://%s/%s/%s", scheme, m.publicEndpoint, m.bucket, objectName), nil
}

// RemoveObject 删除对象。
func (m *MinioStore) RemoveObject(ctx context.Context, objectName string) error {
	return m.client.RemoveObject(ctx, m.bucket, objectName, minio.RemoveObjectOptions{})
}

// Download 下载对象，返回读取流。调用方用完需 Close。
func (m *MinioStore) Download(ctx context.Context, objectName string) (io.ReadCloser, error) {
	obj, err := m.client.GetObject(ctx, m.bucket, objectName, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("下载对象 %s 失败: %w", objectName, err)
	}
	return obj, nil
}

// ListObjects 列出桶内指定前缀（递归）下的所有对象及其最后修改时间。
// 供孤儿对象清理使用：扫描全桶课程对象，找出未被课程引用的残留。
func (m *MinioStore) ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error) {
	infos := make([]ObjectInfo, 0)
	for obj := range m.client.ListObjects(ctx, m.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("列出对象 %s 失败: %w", obj.Key, obj.Err)
		}
		infos = append(infos, ObjectInfo{Name: obj.Key, LastModified: obj.LastModified})
	}
	return infos, nil
}

// publicURL 用 PublicEndpoint 拼一个浏览器可直接访问的对象 URL。
func (m *MinioStore) publicURL(objectName string) string {
	scheme := "http"
	if m.useSSL {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, m.publicEndpoint, m.bucket, objectName)
}

// PresignPutObject 生成前端直传预签名 URL。
// 用 presignClient（publicEndpoint）签名，保证 URL 的 host 和签名一致，
// 否则容器内用 minio:9000 签出来再改 host，浏览器 PUT 会 SignatureDoesNotMatch。
func (m *MinioStore) PresignPutObject(ctx context.Context, objectName, contentType string, expiry time.Duration) (string, error) {
	u, err := m.presignClient.PresignedPutObject(ctx, m.bucket, objectName, expiry)
	if err != nil {
		return "", fmt.Errorf("生成预签名上传 URL 失败: %w", err)
	}
	return u.String(), nil
}

// ComposeObjects 把多个已存在的源对象按顺序合并成 dst。
// 分片上传场景：前端把每片 PUT 到 {objectName}.part1/.part2/...，
// 后端调本方法合并成最终视频，再用 RemoveObject 清掉 parts。
func (m *MinioStore) ComposeObjects(ctx context.Context, dst string, srcs []string) (string, error) {
	if len(srcs) == 0 {
		return "", fmt.Errorf("没有可合并的源对象")
	}
	srcOpts := make([]minio.CopySrcOptions, 0, len(srcs))
	for _, s := range srcs {
		srcOpts = append(srcOpts, minio.CopySrcOptions{
			Bucket: m.bucket,
			Object: s,
		})
	}
	dstOpts := minio.CopyDestOptions{
		Bucket: m.bucket,
		Object: dst,
	}
	if _, err := m.client.ComposeObject(ctx, dstOpts, srcOpts...); err != nil {
		return "", fmt.Errorf("合并对象失败: %w", err)
	}
	return m.publicURL(dst), nil
}
