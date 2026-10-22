package course

import (
	"context"
	"errors"

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
// MinIO 里的封面 / 视频文件不在这里同步删除，交给后台孤儿清理器兜底。
func (l *DeleteCourseLogic) Delete(req *DeleteCourseRequest) (resp *DeleteCourseResponse, err error) {
	if req.Id <= 0 {
		return nil, errors.New("课程 id 不合法")
	}

	if err := store.DeleteCourse(req.Id); err != nil {
		return nil, err
	}

	return &DeleteCourseResponse{Id: req.Id}, nil
}
