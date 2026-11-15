package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type DeleteCourseLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteCourseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteCourseLogic {
	return &DeleteCourseLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Delete 课程删除业务逻辑：
// 校验 id → 从课程存储删除该课程。
// MinIO 里的封面 / 视频 / 讲义文件不在这里同步删除，交给后台孤儿清理器兜底；
// 但知识库索引必须先清掉：切片留在 Mongo/Chroma 里，AI 会继续引用已删除课程的讲义。
func (l *DeleteCourseLogic) Delete(req *course.DeleteCourseRequest) (resp *course.DeleteCourseResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	// 删课前先把资源 id 记下来（删完就查不到了），用于清理知识库
	var resourceIds []int
	if c, gerr := course.Store.GetCourse(req.Id); gerr == nil && c != nil {
		for _, r := range c.Resources {
			if r != nil && r.Id > 0 {
				resourceIds = append(resourceIds, r.Id)
			}
		}
	}

	if err := course.Store.DeleteCourse(req.Id); err != nil {
		return nil, err
	}
	// 列表缓存失效，避免删掉的课还在首页挂 60 秒
	course.InvalidateListCache(l.svcCtx.Redis)

	for _, rid := range resourceIds {
		triggerKnowledgePurge(l.svcCtx, l.ctx, int64(req.Id), int64(rid))
	}

	return &course.DeleteCourseResponse{Id: req.Id}, nil
}
