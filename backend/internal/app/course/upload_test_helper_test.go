package course

import (
	"context"
	"errors"
	"io"

	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

// fakeOss 内存版对象存储（测试专用）：
// 记录上传/删除的对象，供上传逻辑测试注入，不依赖真实 MinIO 服务。
type fakeOss struct {
	objects map[string]string // objectName -> url
	removed []string          // 被删除的对象名（按顺序记录）
	putErr  error             // 恒定失败：让每次 PutObject 都失败
	failOn  int               // 第几次 PutObject 失败（从 1 起；0 表示不按次数失败）
	putCall int               // 已执行的 PutObject 次数
}

func newFakeOss() *fakeOss {
	return &fakeOss{objects: make(map[string]string)}
}

// PutObject 模拟上传：记录对象并返回假 URL。
func (f *fakeOss) PutObject(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	f.putCall++
	if f.putErr != nil {
		return "", f.putErr
	}
	if f.failOn > 0 && f.putCall == f.failOn {
		return "", errors.New("模拟上传失败")
	}

	url := "http://fake-minio:9000/learningplatform/" + objectName
	f.objects[objectName] = url

	// 模拟上传：把流读完（验证 reader 内容能被正常消费）
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return "", err
	}
	return url, nil
}

// RemoveObject 模拟删除：记录到 removed 列表，供断言回滚行为。
func (f *fakeOss) RemoveObject(ctx context.Context, objectName string) error {
	f.removed = append(f.removed, objectName)
	delete(f.objects, objectName)
	return nil
}

// newUploadTestSvcCtx 构造带假对象存储和上传配置的 ServiceContext。
// 复用 createlogic_test.go 里的 newTestSvcCtx，再注入上传相关依赖。
func newUploadTestSvcCtx() *svc.ServiceContext {
	ctx := newTestSvcCtx()
	ctx.Config.Upload = config.UploadConf{
		MaxCoverBytes: 5 << 20,   // 5MB
		MaxVideoBytes: 200 << 20, // 200MB
	}
	ctx.Oss = newFakeOss()
	return ctx
}
