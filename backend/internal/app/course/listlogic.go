package course

import (
	"context"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLogic {
	return &ListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 课程列表业务逻辑：
// 从内存课程存储取出全部课程，原样返回给前端展示。
func (l *ListLogic) List() (resp *ListResponse, err error) {
	return &ListResponse{Courses: store.List()}, nil
}
