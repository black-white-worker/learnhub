package course

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"learningplatform/internal/svc"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

type ResourceUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewResourceUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResourceUploadLogic {
	return &ResourceUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UploadResource 课程文档资源上传业务逻辑：
// 1. 课程必须存在、对象存储必须可用
// 2. 校验大小与扩展名白名单（文档不做魔数校验，Office 格式头复杂，白名单已足够教学场景）
// 3. 上传 MinIO（对象名按课程维度组织）
// 4. 写回课程资源列表
func (l *ResourceUploadLogic) UploadResource(req *UploadResourceRequest) (resp *UploadResourceResponse, err error) {
	// 1. 课程必须存在
	if _, err := store.GetCourse(req.CourseId); err != nil {
		return nil, err
	}

	// 2. 对象存储必须可用
	if l.svcCtx.Oss == nil {
		return nil, errors.New("对象存储未初始化，请检查 MinIO 配置")
	}

	// 3. 大小与空文件校验
	if len(req.FileBytes) == 0 {
		return nil, errors.New("资源文件不能为空")
	}
	maxBytes := effectiveMaxResourceBytes(l.svcCtx)
	if int64(len(req.FileBytes)) > maxBytes {
		return nil, fmt.Errorf("资源文件超过大小上限（%dMB）", maxBytes>>20)
	}

	// 4. 扩展名校验
	ext := strings.ToLower(filepath.Ext(req.FileName))
	if !isAllowedResource(ext) {
		return nil, errors.New("资源格式不支持（仅支持 pdf / ppt / pptx / doc / docx）")
	}

	// 5. 上传 MinIO。对象名用 uuid 避免同名覆盖
	objectName := fmt.Sprintf("course/%d/resources/%s%s", req.CourseId, uuid.NewString(), ext)
	url, err := l.svcCtx.Oss.PutObject(l.ctx, objectName, bytes.NewReader(req.FileBytes),
		int64(len(req.FileBytes)), resourceContentTypeByExt(ext))
	if err != nil {
		return nil, err
	}

	// 6. 写回课程资源列表
	res := &Resource{
		Name:       req.FileName,
		Url:        url,
		Size:       int64(len(req.FileBytes)),
		Type:       resourceTypeByExt(ext),
		UploadedAt: time.Now().Format(time.RFC3339),
	}
	if err := store.AddResource(req.CourseId, res); err != nil {
		// 写元信息失败：删掉刚上传的对象，保持一致
		_ = l.svcCtx.Oss.RemoveObject(l.ctx, objectName)
		return nil, err
	}

	return &UploadResourceResponse{CourseId: req.CourseId, Resource: res}, nil
}
