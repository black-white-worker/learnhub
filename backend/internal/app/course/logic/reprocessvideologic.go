package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ReprocessVideoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReprocessVideoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReprocessVideoLogic {
	return &ReprocessVideoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Reprocess 把后处理失败（process_status=failed）的视频重新排队。
//
// 队列上限是 3 次自动重试，超过就置 failed 并停下 —— 这个接口给人工兜底：
// 比如 ffmpeg 缺失修好了、或者源文件本来就只是网络抖动失败。
// 重试次数与错误信息一并清零，worker 下一轮（或本次唤醒）就会重新认领。
func (l *ReprocessVideoLogic) Reprocess(req *course.ReprocessVideoRequest) (*course.ReprocessVideoResponse, error) {
	if req.CourseId <= 0 || req.VideoId <= 0 {
		return nil, errors.New("参数不合法")
	}
	if err := course.Store.RequeueVideo(req.CourseId, req.VideoId); err != nil {
		return nil, err
	}
	// 不用等 1 分钟轮询：唤醒 worker 立刻处理（队列空闲时几秒内开始转码）
	course.WakeVideoProcessor()
	l.Infof("视频 %d（课程 %d）已重新排队后处理", req.VideoId, req.CourseId)
	return &course.ReprocessVideoResponse{
		CourseId:      req.CourseId,
		VideoId:       req.VideoId,
		ProcessStatus: model.VideoProcessPending,
	}, nil
}
