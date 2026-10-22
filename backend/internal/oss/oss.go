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

	// ListObjects 列出指定前缀下的所有对象（递归），供孤儿对象清理扫描。
	ListObjects(ctx context.Context, prefix string) ([]ObjectInfo, error)
}

// MinioStore 基于 MinIO 的实现。
type MinioStore struct {
	client         *minio.Client
	bucket         string // 默认桶名
	endpoint       string // 服务地址，用于拼 URL
	publicEndpoint string // 对外访问地址（浏览器可访问），为空时退回 endpoint
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
