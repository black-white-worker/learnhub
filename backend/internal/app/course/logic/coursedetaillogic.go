package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/model"
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

// Detail 课程详情。
//
// 详情本身是公开的（首页/分享链接要能打开），但媒体 URL 分两种处理：
//   - 封面属于宣传素材：直接返回预签名链接；
//   - 视频：只有通过媒体鉴权（登录 + 选课关系）才返回预签名 URL，
//     否则 Url 置空并给出 canWatch=false + 原因，前端渲染"登录/加入班级后学习"。
func (l *CourseDetailLogic) Detail(req *course.CourseDetailRequest, actor *model.SessionUser) (resp *course.CourseDetailResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	c, err := course.Store.GetCourse(req.Id)
	if err != nil {
		return nil, course.ErrNotFound("课程不存在")
	}

	access := course.ResolveMediaAccess(l.svcCtx, actor, c)

	// 副本 + 媒体 URL 重写：私有桶开启时用预签名 URL 替换直链，
	// 不动 Store 里的原始对象（内存实现返回的是指针，改它等于改全局状态）。
	bucket := l.svcCtx.Config.Minio.Bucket
	videos := make([]*course.Video, 0, len(c.Videos))
	for _, v := range c.Videos {
		cp := *v
		if access.Allowed {
			cp.Url = course.MediaURL(l.svcCtx, videoObjectName(v, bucket), v.Url)
		} else {
			// 没有权限的人拿不到可播放地址，避免"详情页白送直链"
			cp.Url = ""
		}
		videos = append(videos, &cp)
	}

	coverURL := course.MediaURL(l.svcCtx, course.ObjectNameFromURL(c.CoverUrl, bucket), c.CoverUrl)
	coverThumb := course.MediaURL(l.svcCtx, course.ObjectNameFromURL(c.CoverThumbUrl, bucket), c.CoverThumbUrl)

	return &course.CourseDetailResponse{
		Id:            c.Id,
		Name:          c.Name,
		Intro:         c.Intro,
		Teacher:       c.Teacher,
		CreatedBy:     c.CreatedBy,
		CoverUrl:      coverURL,
		CoverThumbUrl: coverThumb,
		Videos:        videos,
		CanWatch:      access.Allowed,
		MediaReason:   access.Reason,
	}, nil
}
