package logic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/course"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/model"
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
		return nil, course.CourseLookupError(req.CourseId, err)
	}
	// 只有课程创建者/管理员能往这门课上传视频（改造前只校验"已登录"）
	if err := course.AuthorizeAs(cu.Username, cu.Role, req.CourseId, authz.CourseUpload); err != nil {
		return nil, err
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
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: fmt.Sprintf("视频超过大小上限（%dMB）", maxBytes>>20)}
	}
	// 每日上传配额：分片直传绕过普通上传 handler，必须在这里自己预占。
	// 配额在 init 时按声明大小一次性计入，complete/abort 不再变动（见 quota 包注释）。
	if err := l.svcCtx.Quota.Reserve(cu.Id, req.FileSize); err != nil {
		return nil, err
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
		// 会话没建起来就归还预占的额度，避免用户白扣一次配额
		l.svcCtx.Quota.Release(cu.Id, req.FileSize)
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

// maxSignBatch 一次批量签名最多签多少片。
// 上限是为了挡住"一把要 10000 个 URL"的滥用（每个 URL 虽只花本地 HMAC，
// 但响应体也会变成几十 KB）。前端按窗口申请，正常不会撞到这个上限。
const maxSignBatch = 100

// SignParts 批量给若干片签直传 URL。
// 每片存为独立对象：{objectName}.part{N}，前端直接 PUT 字节流。
//
// 为什么批量：预签名是本地 HMAC 计算，不碰 MinIO，一次签 50 片和签 1 片
// 的服务器代价几乎一样，但少 49 次 HTTP 往返。
func (l *MultipartUploadLogic) SignParts(req *course.SignPartsRequest, cu *class.CtxUser) (*course.SignPartsResponse, error) {
	sess, bizErr := l.requireSession(req.UploadID, cu)
	if bizErr != nil {
		return nil, bizErr
	}
	if len(req.PartNumbers) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "partNumbers 不能为空"}
	}
	if len(req.PartNumbers) > maxSignBatch {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: fmt.Sprintf("一次最多签 %d 片", maxSignBatch)}
	}
	seen := make(map[int]bool, len(req.PartNumbers))
	numbers := make([]int, 0, len(req.PartNumbers))
	for _, n := range req.PartNumbers {
		// 片号 1~10000 是 MinIO/S3 的分片号上限；重复的直接去重，
		// 免得前端记账错一次就返回两个同片号 URL。
		if n < 1 || n > 10000 {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "分片号非法（1~10000）"}
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		numbers = append(numbers, n)
	}
	sort.Ints(numbers)

	parts := make([]course.SignedPart, 0, len(numbers))
	for _, n := range numbers {
		partObjectName := fmt.Sprintf("%s.part%d", sess.ObjectName, n)
		url, err := l.svcCtx.Oss.PresignPutObject(l.ctx, partObjectName, sess.ContentType, presignExpiry)
		if err != nil {
			return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "生成直传 URL 失败: " + err.Error()}
		}
		parts = append(parts, course.SignedPart{PartNumber: n, URL: url})
	}
	return &course.SignPartsResponse{Parts: parts, ExpireIn: int(presignExpiry.Seconds())}, nil
}

// Complete 合并分片：把 part1..partN 用 ComposeObject 合成最终 objectName，
// 然后删除临时 parts，把视频元信息挂到课程下。
//
// 合并清单以对象存储里真实存在的分片为准，不信客户端声明：
// 少合一片 = 视频尾部丢字节且永远发现不了，多合一片 = 直接报错。
// 同时校验分片总字节 == 会话声明的文件大小，挡住"断点续传拼错/半截分片"。
func (l *MultipartUploadLogic) Complete(req *course.CompleteMultipartRequest, cu *class.CtxUser) (*course.CompleteMultipartResponse, error) {
	sess, bizErr := l.requireSession(req.UploadID, cu)
	if bizErr != nil {
		return nil, bizErr
	}
	if len(req.Parts) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "分片列表为空"}
	}

	present, err := l.sessionPartSizes(sess)
	if err != nil {
		return nil, err
	}
	numbers, bizErr := checkSessionParts(present, sess.FileSize)
	if bizErr != nil {
		return nil, bizErr
	}

	srcs := make([]string, 0, len(numbers))
	for _, n := range numbers {
		srcs = append(srcs, fmt.Sprintf("%s.part%d", sess.ObjectName, n))
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
	// 分片合并完只是"文件传上来了"，faststart 交给后处理队列（同普通上传），
	// 这里不再开裸协程转码。
	course.WakeVideoProcessor()
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
	// 用户主动放弃：Init 时按声明大小预占的当天额度要归还。
	// 分片对象已经删干净了，留着占用等于让用户为没落地的字节买单。
	if l.svcCtx.Quota != nil {
		l.svcCtx.Quota.Release(sess.CreatedBy, sess.FileSize)
	}
	return nil
}

// Status 查询分片会话进度，供断点续传使用。
//
// 前端刷新页面 / 断网重连后拿 localStorage 里的 uploadId 调这里，
// 服务端以 MinIO 里真实存在的 part 对象为准返回已完成分片 ——
// 不信浏览器记账，因为记账可能正好是丢的那一半（传完没记上 / 记上没传完）。
func (l *MultipartUploadLogic) Status(req *course.MultipartStatusRequest, cu *class.CtxUser) (*course.MultipartStatusResponse, error) {
	if req.UploadID == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "uploadId 不能为空"}
	}
	if l.svcCtx.UploadStore == nil || l.svcCtx.Oss == nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "上传服务未初始化"}
	}
	sess, err := l.svcCtx.UploadStore.GetByUploadID(req.UploadID)
	if err != nil {
		// 会话查不到 = 不该再续传，前端清掉本地记账重新 init
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "上传会话不存在或已过期"}
	}
	if sess.CreatedBy != cu.Id && cu.Role != "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "无权查看他人的上传会话"}
	}

	// 已完成分片以对象存储里的实际内容为准
	present, err := l.sessionPartSizes(sess)
	if err != nil {
		return nil, err
	}
	parts := make([]course.UploadedPart, 0, len(present))
	var uploaded int64
	for n, size := range present {
		parts = append(parts, course.UploadedPart{PartNumber: n, Size: size})
		uploaded += size
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })

	// resumable：状态还是 init 且没过期才能续传。
	// 过期会话的 parts 会被孤儿清理器回收，续传只会合出半截视频。
	resumable := sess.Status == "init" && time.Now().Before(sess.ExpireAt)
	status := sess.Status
	if !resumable && status == "init" {
		status = "expired"
	}
	return &course.MultipartStatusResponse{
		UploadID:  sess.UploadID,
		Status:    status,
		CourseId:  int(sess.CourseId),
		FileName:  sess.FileName,
		FileSize:  sess.FileSize,
		ChunkSize: defaultChunkSize,
		Uploaded:  uploaded,
		Parts:     parts,
		ExpireAt:  sess.ExpireAt.Format(time.RFC3339),
		Resumable: resumable,
	}, nil
}

// sessionPartSizes 列出会话在 MinIO 里已经存在的分片：片号 → 字节数。
// 查询失败一定返回错误，调用方不能"当成没有分片"继续合并。
func (l *MultipartUploadLogic) sessionPartSizes(sess *model.UploadSessionRow) (map[int]int64, error) {
	if l.svcCtx.Oss == nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "对象存储未初始化"}
	}
	infos, err := l.svcCtx.Oss.ListObjects(l.ctx, sess.ObjectName+".part")
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "查询已传分片失败: " + err.Error()}
	}
	sizes := make(map[int]int64, len(infos))
	for _, obj := range infos {
		if n, ok := parsePartNumber(obj.Name, sess.ObjectName); ok {
			sizes[n] = obj.Size
		}
	}
	return sizes, nil
}

// checkSessionParts 合并前的四道校验（纯函数，可单测）：
//  1. 至少收到一片；
//  2. 片号必须从 1 开始连续 —— 断号会合出"能播放但中间缺字节"的视频，事后永远发现不了；
//  3. 分片合计字节 == 会话声明的文件大小 —— 挡住半截分片和续传拼错；
//  4. 除最后一片外每片 ≥ 5MB（MinIO ComposeObject 硬要求），
//     前端切片切错时在这里拦下，比让 MinIO 抛一句看不懂的 500 好。
//
// 返回按序排列的片号。
func checkSessionParts(present map[int]int64, fileSize int64) ([]int, *class.BizError) {
	if len(present) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "服务端还没有收到任何分片，无法合并"}
	}
	numbers := make([]int, 0, len(present))
	var total int64
	for n, size := range present {
		numbers = append(numbers, n)
		total += size
	}
	sort.Ints(numbers)

	for i, n := range numbers {
		if n != i+1 {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: fmt.Sprintf("第 %d 片缺失，请续传后再合并", i+1)}
		}
	}
	if total != fileSize {
		return nil, &class.BizError{
			Code: http.StatusBadRequest,
			Msg:  fmt.Sprintf("分片合计 %d 字节，与声明的文件大小 %d 不一致，请重新上传", total, fileSize),
		}
	}
	for _, n := range numbers[:len(numbers)-1] {
		if present[n] < defaultChunkSize {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: fmt.Sprintf("第 %d 片不足 %d 字节，分片大小不合法", n, defaultChunkSize)}
		}
	}
	return numbers, nil
}

// parsePartNumber 从分片对象名 {objectName}.part{N} 里取片号。
// 前缀不匹配或后缀不是数字（比如清理器留下的临时对象）一律忽略 ——
// 这里宁可少报一片（前端会重传），也不能把别的对象当成分片报上去。
func parsePartNumber(objectName, baseObjectName string) (int, bool) {
	prefix := baseObjectName + ".part"
	if !strings.HasPrefix(objectName, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(objectName, prefix))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
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
