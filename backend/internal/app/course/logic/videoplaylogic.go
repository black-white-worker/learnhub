package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type VideoPlayLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewVideoPlayLogic(ctx context.Context, svcCtx *svc.ServiceContext) *VideoPlayLogic {
	return &VideoPlayLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Play 返回某课程下某个视频的播放信息。
// 视频 URL 已经是 faststart 版本（上传时异步生成），浏览器可边下边播、拖动进度条。
func (l *VideoPlayLogic) Play(req *course.PlayVideoRequest) (resp *course.PlayVideoResponse, err error) {
	if req.CourseId <= 0 || req.VideoId <= 0 {
		return nil, errors.New("参数不合法")
	}
	v, err := course.Store.FindVideo(req.CourseId, req.VideoId)
	if err != nil {
		return nil, err
	}
	// 私有桶：返回 30 分钟有效的预签名 URL；公开桶则原样返回直链。
	// 返回副本，避免改动 Store 里的原始对象（内存实现是指针）。
	cp := *v
	cp.Url = course.MediaURL(l.svcCtx, videoObjectName(v, l.svcCtx.Config.Minio.Bucket), v.Url)
	return &course.PlayVideoResponse{Video: &cp}, nil
}

// videoObjectName 决定给视频签哪个对象，优先级：
//  1. 从 url 反解出的对象路径 —— 上传后如果转成了 faststart 版本，
//     库里的 url 指向 faststart 对象，签它才能保留"边下边播"；
//  2. ObjectName（源文件）—— url 不是本站直链时兜底。
//
// 只签 ObjectName 会导致私有桶下总是播放源文件（moov 在尾部、拖动卡顿）。
func videoObjectName(v *course.Video, bucket string) string {
	if name := course.ObjectNameFromURL(v.Url, bucket); name != "" {
		return name
	}
	return v.ObjectName
}
