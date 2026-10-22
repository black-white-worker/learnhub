package course

import (
	"context"
	"errors"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type CourseDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCourseDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CourseDetailLogic {
	return &CourseDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Detail 课程详情业务逻辑：
// 根据课程 id 取出完整课程（含封面、视频列表），组装成详情响应。
// 课程不存在时返回错误，前端据此跳回列表页。
func (l *CourseDetailLogic) Detail(req *CourseDetailRequest) (resp *CourseDetailResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	c, err := store.GetCourse(req.Id)
	if err != nil {
		return nil, err
	}

	// 视频列表为空时也返回空切片而不是 nil，方便前端直接 v-for
	videos := c.Videos
	if videos == nil {
		videos = []*Video{}
	}

	return &CourseDetailResponse{
		Id:            c.Id,
		Name:          c.Name,
		Intro:         c.Intro,
		Teacher:       c.Teacher,
		CoverUrl:      c.CoverUrl,
		CoverThumbUrl: c.CoverThumbUrl,
		Videos:        videos,
	}, nil
}
