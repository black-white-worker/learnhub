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
func (l *ResourceListLogic) List(req *course.ListResourcesRequest) (resp *course.ListResourcesResponse, err error) {
	resources, err := course.Store.ListResources(req.CourseId)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		resources = []*course.Resource{}
	}
	return &course.ListResourcesResponse{CourseId: req.CourseId, Resources: resources}, nil
}
