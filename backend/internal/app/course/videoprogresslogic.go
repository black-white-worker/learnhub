package course

import (
	"context"
	"strconv"

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

// progressKey 拼 Redis key：一门课程下的一个视频记一条播放位置。
// 教学版按视频记（单用户场景），不细分用户。
func progressKey(courseId, videoId int) string {
	return "course:video:progress:" + strconv.Itoa(courseId) + ":" + strconv.Itoa(videoId)
}

// Save 保存播放位置（秒）。Redis 不可用时只记日志，不影响观看。
func (l *VideoProgressLogic) Save(req *SaveProgressRequest) (resp *SaveProgressResponse, err error) {
	key := progressKey(req.CourseId, req.VideoId)
	if l.svcCtx.Redis != nil {
		if redisErr := l.svcCtx.Redis.Set(key, strconv.Itoa(req.Position)); redisErr != nil {
			l.Logger.Errorf("保存播放进度失败 key=%s: %v", key, redisErr)
		}
	}
	return &SaveProgressResponse{Position: req.Position}, nil
}

// Get 读取上次记住的播放位置（秒）。没有记录返回 0。
func (l *VideoProgressLogic) Get(req *GetProgressRequest) (resp *GetProgressResponse, err error) {
	key := progressKey(req.CourseId, req.VideoId)
	position := 0
	if l.svcCtx.Redis != nil {
		if val, redisErr := l.svcCtx.Redis.Get(key); redisErr == nil && val != "" {
			if p, parseErr := strconv.Atoi(val); parseErr == nil && p > 0 {
				position = p
			}
		}
	}
	return &GetProgressResponse{Position: position}, nil
}
