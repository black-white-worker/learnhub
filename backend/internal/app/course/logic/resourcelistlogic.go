package logic

import (
	"context"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ResourceListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewResourceListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResourceListLogic {
	return &ResourceListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 列出某课程的全部文档资源。空列表时返回空切片，方便前端直接 v-for。
//
// 私有桶开启时把每个资源的 URL 换成预签名 URL（30 分钟有效），
// 避免讲义直链被转发/长期盗用；公开桶（默认）行为不变。
func (l *ResourceListLogic) List(req *course.ListResourcesRequest) (resp *course.ListResourcesResponse, err error) {
	resources, err := course.Store.ListResources(req.CourseId)
	if err != nil {
		return nil, err
	}
	out := make([]*course.Resource, 0, len(resources))
	for _, r := range resources {
		cp := *r
		cp.Url = course.MediaURL(l.svcCtx, r.ObjectName, r.Url)
		out = append(out, &cp)
	}
	return &course.ListResourcesResponse{CourseId: req.CourseId, Resources: out}, nil
}
