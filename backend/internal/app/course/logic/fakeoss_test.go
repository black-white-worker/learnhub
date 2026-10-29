package logic

// 本文件是 course/logic 包测试专用的假对象存储。
//
// 背景：fakeOss 原先只定义在 course 包（upload_test_helper_test.go），
// 而这些用例在 logic 包里，跨包引用不到（_test.go 不属于可导入的 API），
// 导致整个 logic 测试包编译失败。这里放一份同构实现，接口与行为保持一致。

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"learningplatform/internal/platform/oss"
)

type fakeOss struct {
	objects   map[string]string
	createdAt map[string]time.Time
	contents  map[string][]byte
	removed   []string
	putErr    error
	failOn    int
	putCall   int
}

func newFakeOss() *fakeOss {
	return &fakeOss{
		objects:   make(map[string]string),
		createdAt: make(map[string]time.Time),
		contents:  make(map[string][]byte),
	}
}

func (f *fakeOss) PutObject(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) (string, error) {
	f.putCall++
	if f.putErr != nil {
		return "", f.putErr
	}
	if f.failOn > 0 && f.putCall == f.failOn {
		return "", errors.New("模拟上传失败")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	url := "http://fake-minio:9000/learningplatform/" + objectName
	f.objects[objectName] = url
	f.createdAt[objectName] = time.Now()
	f.contents[objectName] = data
	return url, nil
}

func (f *fakeOss) Download(ctx context.Context, objectName string) (io.ReadCloser, error) {
	data, ok := f.contents[objectName]
	if !ok {
		return nil, errors.New("对象不存在: " + objectName)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeOss) RemoveObject(ctx context.Context, objectName string) error {
	f.removed = append(f.removed, objectName)
	delete(f.objects, objectName)
	delete(f.createdAt, objectName)
	return nil
}

func (f *fakeOss) ListObjects(ctx context.Context, prefix string) ([]oss.ObjectInfo, error) {
	infos := make([]oss.ObjectInfo, 0)
	for name := range f.objects {
		if strings.HasPrefix(name, prefix) {
			infos = append(infos, oss.ObjectInfo{Name: name, LastModified: f.createdAt[name]})
		}
	}
	return infos, nil
}

func (f *fakeOss) PresignPutObject(ctx context.Context, objectName, contentType string, expiry time.Duration) (string, error) {
	return "http://fake-presign/" + objectName, nil
}

func (f *fakeOss) ComposeObjects(ctx context.Context, dst string, srcs []string) (string, error) {
	f.objects[dst] = "http://fake-minio:9000/learningplatform/" + dst
	return f.objects[dst], nil
}

// PresignGetObject 预签名下载（私有桶改造新增的方法）。
func (f *fakeOss) PresignGetObject(ctx context.Context, objectName string, expiry time.Duration) (string, error) {
	return "http://fake-presign-get/" + objectName, nil
}
