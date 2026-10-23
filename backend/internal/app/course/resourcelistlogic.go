package course

import (
	"context"

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
func (l *ResourceListLogic) List(req *ListResourcesRequest) (resp *ListResourcesResponse, err error) {
	resources, err := store.ListResources(req.CourseId)
	if err != nil {
		return nil, err
	}
	if resources == nil {
		resources = []*Resource{}
	}
	return &ListResourcesResponse{CourseId: req.CourseId, Resources: resources}, nil
}
