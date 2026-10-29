package logic

// 课程视频学习进度的读写（对标 Open edX progress 模型）。
//
// 改造前：进度存在 Redis，key = course:video:progress:{courseId}:{videoId}，
// 不带用户 id —— 全班共用一个进度条，且完全无法做学情分析。
//
// 现在：按 (user_id, video_id) 落 MySQL，记录
//   - position_seconds  上次看到哪（续播用）
//   - watched_seconds   累计有效观看时长（前端只上报"真实播放过的增量"，拖动不计）
//   - duration_seconds  视频总时长（播放器上报，服务端没有可靠媒资时长）
//   - completed         是否看完（累计观看 >= 时长 90%）
//
// 兼容：没有用户上下文（例如老单测）或没有 MySQL 数据层时，退回原来的行为，
// 只回显 position，不报错 —— 进度是辅助功能，绝不能因为它挂掉而让人看不了视频。

import (
	"context"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type VideoProgressLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewVideoProgressLogic(ctx context.Context, svcCtx *svc.ServiceContext) *VideoProgressLogic {
	return &VideoProgressLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Save 上报播放进度。
// req.WatchedDelta 是"上次上报以来真实播放的秒数"；Position 是播放头位置。
func (l *VideoProgressLogic) Save(req *course.SaveProgressRequest) (resp *course.SaveProgressResponse, err error) {
	if req.UserId <= 0 || l.svcCtx.CourseStore == nil {
		// 无法按用户落库：只回显位置（保持接口可用，不阻断观看）
		return &course.SaveProgressResponse{Position: req.Position}, nil
	}

	p, err := l.svcCtx.CourseStore.SaveVideoProgress(
		req.UserId, int64(req.CourseId), int64(req.VideoId),
		req.Position, req.WatchedDelta, req.Duration,
	)
	if err != nil {
		// 进度是辅助功能：写失败不抛给前端，只记日志
		l.Logger.Errorf("保存学习进度失败 user=%d video=%d: %v", req.UserId, req.VideoId, err)
		return &course.SaveProgressResponse{Position: req.Position}, nil
	}

	return &course.SaveProgressResponse{
		Position:       p.PositionSeconds,
		WatchedSeconds: p.WatchedSeconds,
		Completed:      p.Completed,
	}, nil
}

// Get 读取上次播放位置 + 累计观看进度。
func (l *VideoProgressLogic) Get(req *course.GetProgressRequest) (resp *course.GetProgressResponse, err error) {
	resp = &course.GetProgressResponse{}
	if req.UserId <= 0 || l.svcCtx.CourseStore == nil {
		return resp, nil
	}
	p, err := l.svcCtx.CourseStore.GetVideoProgress(req.UserId, int64(req.VideoId))
	if err != nil {
		l.Logger.Errorf("读取学习进度失败 user=%d video=%d: %v", req.UserId, req.VideoId, err)
		return resp, nil
	}
	if p == nil {
		return resp, nil
	}
	resp.Position = p.PositionSeconds
	resp.WatchedSeconds = p.WatchedSeconds
	resp.DurationSeconds = p.DurationSeconds
	resp.Completed = p.Completed
	return resp, nil
}
