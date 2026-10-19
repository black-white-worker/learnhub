package course

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"learningplatform/internal/svc"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

type VideoUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewVideoUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *VideoUploadLogic {
	return &VideoUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UploadVideos 课程视频上传业务逻辑（一次可传多个文件）：
// 1. 课程必须存在
// 2. 逐个校验：大小上限、扩展名白名单、魔数
// 3. 逐个流式上传 MinIO（不把大视频整读进内存）
// 4. 全部上传成功后才写回课程存储；中途失败回滚已上传的对象
func (l *VideoUploadLogic) UploadVideos(req *UploadVideosRequest) (resp *UploadVideosResponse, err error) {
	// 1. 课程必须存在
	if _, err := store.GetCourse(req.CourseId); err != nil {
		return nil, err
	}

	// 2. 对象存储必须可用
	if l.svcCtx.Oss == nil {
		return nil, errors.New("对象存储未初始化，请检查 MinIO 配置")
	}

	// 3. 至少要有一个文件
	if len(req.Files) == 0 {
		return nil, errors.New("请至少上传一个视频文件")
	}

	maxBytes := effectiveMaxVideoBytes(l.svcCtx)

	// 已成功上传的对象名：中途失败时逐个回滚
	uploadedObjects := make([]string, 0, len(req.Files))
	// 先上传对象、后写存储，避免"元信息写了但对象没传上去"
	videos := make([]*Video, 0, len(req.Files))

	// 4. 逐个处理文件
	for _, f := range req.Files {
		// 4.1 大小校验
		if f.Size <= 0 {
			l.rollback(uploadedObjects)
			return nil, errors.New("存在空视频文件")
		}
		if f.Size > maxBytes {
			l.rollback(uploadedObjects)
			return nil, fmt.Errorf("视频文件 %s 超过大小上限", f.FileName)
		}

		// 4.2 扩展名校验
		ext := strings.ToLower(filepath.Ext(f.FileName))
		if !isAllowedVideo(ext) {
			l.rollback(uploadedObjects)
			return nil, fmt.Errorf("视频文件 %s 扩展名不支持（仅支持 mp4/mov/webm/avi/mkv）", f.FileName)
		}

		// 4.3 魔数校验：先读文件头 16 字节判断真实格式
		head := make([]byte, 16)
		n, readErr := io.ReadFull(f.Reader, head)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			l.rollback(uploadedObjects)
			return nil, readErr
		}
		if n < 12 || !checkVideoMagic(head[:n]) {
			l.rollback(uploadedObjects)
			return nil, fmt.Errorf("视频文件 %s 不是受支持的视频格式", f.FileName)
		}

		// 4.4 流式上传 MinIO。
		// 对象名用 uuid 保证唯一，避免同名文件互相覆盖；
		// 头部 16 字节 + 剩余内容拼回同一个流，保证上传的是完整文件。
		objectName := fmt.Sprintf("course/%d/videos/%s%s", req.CourseId, uuid.NewString(), ext)
		full := io.MultiReader(bytes.NewReader(head[:n]), f.Reader)
		url, putErr := l.svcCtx.Oss.PutObject(l.ctx, objectName, full, f.Size, contentTypeByExt(ext))
		if putErr != nil {
			l.rollback(uploadedObjects)
			return nil, putErr
		}
		uploadedObjects = append(uploadedObjects, objectName)

		// 4.5 记录视频元信息（id 由 store.AddVideos 统一分配）
		videos = append(videos, &Video{
			Name:       f.FileName,
			Url:        url,
			Size:       f.Size,
			UploadedAt: time.Now().Format(time.RFC3339),
		})
	}

	// 5. 全部上传成功后才写回课程存储（AddVideos 会给每个视频分配 id）
	if err := store.AddVideos(req.CourseId, videos); err != nil {
		l.rollback(uploadedObjects)
		return nil, err
	}

	return &UploadVideosResponse{CourseId: req.CourseId, Videos: videos}, nil
}

// rollback 删除本次已上传到 MinIO 的对象（尽力而为，失败只记日志）。
func (l *VideoUploadLogic) rollback(objectNames []string) {
	for _, name := range objectNames {
		if err := l.svcCtx.Oss.RemoveObject(l.ctx, name); err != nil {
			l.Logger.Errorf("回滚删除对象 %s 失败: %v", name, err)
		}
	}
}
