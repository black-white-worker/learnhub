package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/course"
	"learningplatform/internal/model"
	"learningplatform/internal/oss"
	"learningplatform/internal/svc"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

// 默认分片大小 5MB。MinIO ComposeObject 要求除最后一片外每片 ≥ 5MB。
const defaultChunkSize int64 = 5 << 20

// 预签名 URL 有效期
const presignExpiry = 15 * time.Minute

// 会话过期时间（init 后 24 小时未 complete 自动作废）
const sessionTTL = 24 * time.Hour

type MultipartUploadLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMultipartUploadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MultipartUploadLogic {
	return &MultipartUploadLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Init 开启分片上传会话。
// 不再调 MinIO 的 multipart API（minio-go v7 没公开），
// 直接生成 uploadId + objectName，写 DB；前端之后把每片 PUT 到独立对象。
func (l *MultipartUploadLogic) Init(req *course.InitMultipartRequest, cu *class.CtxUser) (*course.InitMultipartResponse, error) {
	if l.svcCtx.Oss == nil {
		return nil, errors.New("对象存储未初始化")
	}
	if _, err := course.Store.GetCourse(req.CourseId); err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "课程不存在"}
	}
	ext := strings.ToLower(filepath.Ext(req.FileName))
	if !course.IsAllowedVideo(ext) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "仅支持 mp4/mov/webm/avi/mkv"}
	}
	maxBytes := course.EffectiveMaxVideoBytes(l.svcCtx)
	if req.FileSize <= 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "文件大小非法"}
	}
	if req.FileSize > maxBytes {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "视频超过大小上限"}
	}

	uploadID := uuid.NewString()
	objectName := fmt.Sprintf("course/%d/videos/%s%s", req.CourseId, uuid.NewString(), ext)
	expireAt := time.Now().Add(sessionTTL)
	if _, err := l.svcCtx.UploadStore.Insert(&model.UploadSessionRow{
		UploadID:    uploadID,
		CourseId:    int64(req.CourseId),
		ObjectName:  objectName,
		FileName:    req.FileName,
		FileSize:    req.FileSize,
		ContentType: course.ContentTypeByExt(ext),
		Status:      "init",
		ExpireAt:    expireAt,
		CreatedBy:   cu.Id,
	}); err != nil {
		return nil, err
	}
	return &course.InitMultipartResponse{
		UploadID:    uploadID,
		ObjectName:  objectName,
		ChunkSize:   defaultChunkSize,
		ExpireAt:    expireAt.Format(time.RFC3339),
		MinPartSize: defaultChunkSize,
	}, nil
}

// SignPart 给第 partNumber 片签一个预签名 PUT URL。
// 每片存为独立对象：{objectName}.part{N}，前端直接 PUT 字节流。
func (l *MultipartUploadLogic) SignPart(req *course.SignPartRequest, cu *class.CtxUser) (*course.SignPartResponse, error) {
	sess, bizErr := l.requireSession(req.UploadID, cu)
	if bizErr != nil {
		return nil, bizErr
	}
	if req.PartNumber <= 0 || req.PartNumber > 10000 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "分片号非法（1~10000）"}
	}
	partObjectName := fmt.Sprintf("%s.part%d", sess.ObjectName, req.PartNumber)
	url, err := l.svcCtx.Oss.PresignPutObject(l.ctx, partObjectName, sess.ContentType, presignExpiry)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "生成直传 URL 失败: " + err.Error()}
	}
	return &course.SignPartResponse{URL: url, PartNumber: req.PartNumber}, nil
}

// Complete 合并分片：把 part1..partN 用 ComposeObject 合成最终 objectName，
// 然后删除临时 parts，把视频元信息挂到课程下。
func (l *MultipartUploadLogic) Complete(req *course.CompleteMultipartRequest, cu *class.CtxUser) (*course.CompleteMultipartResponse, error) {
	sess, bizErr := l.requireSession(req.UploadID, cu)
	if bizErr != nil {
		return nil, bizErr
	}
	if len(req.Parts) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "分片列表为空"}
	}
	// 按 partNumber 升序（ComposeObject 要求按顺序合并）
	parts := make([]oss.MultipartPart, len(req.Parts))
	copy(parts, req.Parts)
	for i := 0; i < len(parts); i++ {
		for j := i + 1; j < len(parts); j++ {
			if parts[j].PartNumber < parts[i].PartNumber {
				parts[i], parts[j] = parts[j], parts[i]
			}
		}
	}

	srcs := make([]string, 0, len(parts))
	for _, p := range parts {
		srcs = append(srcs, fmt.Sprintf("%s.part%d", sess.ObjectName, p.PartNumber))
	}

	videoURL, err := l.svcCtx.Oss.ComposeObjects(l.ctx, sess.ObjectName, srcs)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "合并分片失败: " + err.Error()}
	}

	// 清理临时 parts（尽力而为，失败只记日志）
	for _, s := range srcs {
		if err := l.svcCtx.Oss.RemoveObject(l.ctx, s); err != nil {
			l.Logger.Errorf("清理临时分片 %s 失败: %v", s, err)
		}
	}

	if err := l.svcCtx.UploadStore.MarkCompleted(sess.UploadID, time.Now()); err != nil {
		l.Logger.Errorf("标记上传会话完成失败: %v", err)
	}

	video := &course.Video{
		Name:       sess.FileName,
		Url:        videoURL,
		Size:       sess.FileSize,
		UploadedAt: time.Now().Format(time.RFC3339),
		CourseId:   int(sess.CourseId),
		ObjectName: sess.ObjectName,
	}
	if err := course.Store.AddVideos(int(sess.CourseId), []*course.Video{video}); err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: err.Error()}
	}
	// 异步 faststart 处理（与旧 videouploadlogic 一致）
	go func() {
		defer func() {
			if r := recover(); r != nil {
				l.Logger.Errorf("faststart 处理 panic: %v", r)
			}
		}()
		newURL, ferr := course.ProcessVideoFaststart(context.Background(), l.svcCtx, video)
		if ferr != nil {
			l.Logger.Infof("视频 %d faststart 跳过：%v", video.Id, ferr)
			return
		}
		if ferr := course.Store.MarkVideoFaststart(int(sess.CourseId), video.Id, newURL); ferr != nil {
			l.Logger.Errorf("视频 %d faststart 标记落库失败: %v", video.Id, ferr)
		}
		video.Url = newURL
		video.Faststarted = true
	}()
	return &course.CompleteMultipartResponse{CourseId: int(sess.CourseId), Video: video}, nil
}

// Abort 取消上传：清理已传的 parts，标记会话取消。
func (l *MultipartUploadLogic) Abort(req *course.AbortMultipartRequest, cu *class.CtxUser) error {
	sess, err := l.requireSession(req.UploadID, cu)
	if err != nil {
		return err
	}
	// 扫出所有 part 对象并删除（不知道前端传了几片，按前缀列）
	if infos, err := l.svcCtx.Oss.ListObjects(l.ctx, sess.ObjectName+".part"); err == nil {
		for _, obj := range infos {
			if err := l.svcCtx.Oss.RemoveObject(l.ctx, obj.Name); err != nil {
				l.Logger.Errorf("取消时清理分片 %s 失败: %v", obj.Name, err)
			}
		}
	}
	if err := l.svcCtx.UploadStore.MarkAborted(sess.UploadID); err != nil {
		l.Logger.Errorf("标记上传会话取消失败: %v", err)
	}
	return nil
}

// requireSession 校验会话存在、状态 init、当前用户是创建者。
func (l *MultipartUploadLogic) requireSession(uploadID string, cu *class.CtxUser) (*model.UploadSessionRow, *class.BizError) {
	if uploadID == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "uploadId 不能为空"}
	}
	sess, err := l.svcCtx.UploadStore.GetByUploadID(uploadID)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "上传会话不存在或已过期"}
	}
	if sess.Status == "completed" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "该会话已完成，不能重复操作"}
	}
	if sess.Status == "aborted" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "该会话已取消"}
	}
	if sess.CreatedBy != cu.Id && cu.Role != "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "无权操作他人的上传会话"}
	}
	return sess, nil
}
