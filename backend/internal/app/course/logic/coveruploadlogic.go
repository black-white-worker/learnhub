package logic

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type CoverUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCoverUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CoverUploadLogic {
	return &CoverUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UploadCover 课程封面上传业务逻辑：
// 1. 课程必须存在
// 2. 校验文件大小与图片格式（扩展名白名单 + 魔数）
// 3. 生成缩略图（等比缩放 + 居中裁剪 320x180 JPEG）
// 4. 原图与缩略图上传 MinIO
// 5. 更新课程封面的两个 URL（任一步失败都会回滚已上传的对象）
func (l *CoverUploadLogic) UploadCover(req *course.UploadCoverRequest) (resp *course.UploadCoverResponse, err error) {
	const (
		thumbWidth  = 320
		thumbHeight = 180
	)
	// 1. 课程必须存在
	if _, err := course.Store.GetCourse(req.CourseId); err != nil {
		return nil, err
	}

	// 2. 对象存储必须可用（启动时未连上 MinIO 则为 nil）
	if l.svcCtx.Oss == nil {
		return nil, errors.New("对象存储未初始化，请检查 MinIO 配置")
	}

	// 3. 大小校验（配置未设置时走默认 5MB）
	maxBytes := course.EffectiveMaxCoverBytes(l.svcCtx)
	if len(req.FileBytes) == 0 {
		return nil, errors.New("封面文件不能为空")
	}
	if int64(len(req.FileBytes)) > maxBytes {
		return nil, errors.New("封面文件超过大小上限")
	}

	// 4. 扩展名 + 魔数双重校验
	ext := strings.ToLower(filepath.Ext(req.FileName))
	if !course.IsAllowedImage(ext) {
		return nil, errors.New("封面扩展名不支持（仅支持 jpg/jpeg/png/gif）")
	}
	if !course.CheckImageMagic(req.FileBytes) {
		return nil, errors.New("不是受支持的图片格式（仅支持 JPEG/PNG/GIF）")
	}

	// 5. 生成缩略图
	thumb, err := course.GenerateThumbnail(req.FileBytes, thumbWidth, thumbHeight)
	if err != nil {
		return nil, err
	}

	// 6. 上传原图到 MinIO（对象名按课程维度组织，方便后续管理）
	coverObject := fmt.Sprintf("course/%d/cover/original%s", req.CourseId, ext)
	coverUrl, err := l.svcCtx.Oss.PutObject(l.ctx, coverObject, bytes.NewReader(req.FileBytes), int64(len(req.FileBytes)), course.ContentTypeByExt(ext))
	if err != nil {
		return nil, err
	}

	// 7. 上传缩略图到 MinIO
	thumbObject := fmt.Sprintf("course/%d/cover/thumb.jpg", req.CourseId)
	thumbUrl, err := l.svcCtx.Oss.PutObject(l.ctx, thumbObject, bytes.NewReader(thumb), int64(len(thumb)), "image/jpeg")
	if err != nil {
		// 缩略图上传失败：回滚已上传的原图，避免 MinIO 留下孤儿对象
		_ = l.svcCtx.Oss.RemoveObject(l.ctx, coverObject)
		return nil, err
	}

	// 8. 更新课程封面信息
	if err := course.Store.SetCover(req.CourseId, coverUrl, thumbUrl); err != nil {
		// 存储更新失败：两个对象都删掉，保持"存储元信息"和"对象"一致
		_ = l.svcCtx.Oss.RemoveObject(l.ctx, coverObject)
		_ = l.svcCtx.Oss.RemoveObject(l.ctx, thumbObject)
		return nil, err
	}

	return &course.UploadCoverResponse{
		CourseId:      req.CourseId,
		CoverUrl:      coverUrl,
		CoverThumbUrl: thumbUrl,
	}, nil
}
