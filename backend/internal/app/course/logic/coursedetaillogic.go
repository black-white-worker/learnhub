package logic

import (
	"context"
	"errors"
	"strings"

	"learningplatform/internal/app/course"
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
func (l *CourseDetailLogic) Detail(req *course.CourseDetailRequest) (resp *course.CourseDetailResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	c, err := course.Store.GetCourse(req.Id)
	if err != nil {
		return nil, err
	}

	// 视频列表为空时也返回空切片而不是 nil，方便前端直接 v-for。
	// 这里做的是"副本 + 媒体 URL 重写"：私有桶开启时用预签名 URL 替换直链，
	// 不动 Store 里的原始对象（内存实现返回的是指针，改它等于改全局状态）。
	videos := make([]*course.Video, 0, len(c.Videos))
	for _, v := range c.Videos {
		cp := *v
		cp.Url = course.MediaURL(l.svcCtx, videoObjectName(v), v.Url)
		videos = append(videos, &cp)
	}

	coverURL := course.MediaURL(l.svcCtx, objectNameOfURL(c.CoverUrl), c.CoverUrl)
	coverThumb := course.MediaURL(l.svcCtx, objectNameOfURL(c.CoverThumbUrl), c.CoverThumbUrl)

	return &course.CourseDetailResponse{
		Id:            c.Id,
		Name:          c.Name,
		Intro:         c.Intro,
		Teacher:       c.Teacher,
		CreatedBy:     c.CreatedBy,
		CoverUrl:      coverURL,
		CoverThumbUrl: coverThumb,
		Videos:        videos,
	}, nil
}

// objectNameOfURL 从公开直链里反解出对象路径：
// http://127.0.0.1:9020/learningplatform/course/1/cover/x.jpg -> course/1/cover/x.jpg
//
// 为什么需要反解：封面的对象路径没有单独字段落库（只有 cover_url），
// 私有桶签名必须要有 objectName；取不到就返回空串，MediaURL 会退回原 URL。
func objectNameOfURL(raw string) string {
	if raw == "" {
		return ""
	}
	const marker = "/learningplatform/"
	if i := strings.Index(raw, marker); i >= 0 {
		return raw[i+len(marker):]
	}
	// 桶名可配置，这里退化处理：取 URL 路径里 course/ 之后的部分
	if i := strings.Index(raw, "/course/"); i >= 0 {
		return strings.TrimPrefix(raw[i+1:], "/")
	}
	return ""
}
