package course

import (
	"context"
	"errors"

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
func (l *VideoPlayLogic) Play(req *PlayVideoRequest) (resp *PlayVideoResponse, err error) {
	if req.CourseId <= 0 || req.VideoId <= 0 {
		return nil, errors.New("参数不合法")
	}
	v, err := store.FindVideo(req.CourseId, req.VideoId)
	if err != nil {
		return nil, err
	}
	return &PlayVideoResponse{Video: v}, nil
}
