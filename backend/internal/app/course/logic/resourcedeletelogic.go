package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ResourceDeleteLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewResourceDeleteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ResourceDeleteLogic {
	return &ResourceDeleteLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Delete 删除课程下的一个资源。
// 这里只删内存里的资源元信息；MinIO 里的文件对象由后台孤儿清理器兜底回收，
// 与课程删除 / 视频删除的策略保持一致。
func (l *ResourceDeleteLogic) Delete(req *course.DeleteResourceRequest) (resp *course.DeleteResourceResponse, err error) {
	if req.CourseId <= 0 || req.ResourceId <= 0 {
		return nil, errors.New("参数不合法")
	}
	if err := course.Store.DeleteResource(req.CourseId, req.ResourceId); err != nil {
		return nil, err
	}
	return &course.DeleteResourceResponse{CourseId: req.CourseId, ResourceId: req.ResourceId}, nil
}
