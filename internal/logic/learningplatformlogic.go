// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"
	"fmt"

	"learningplatform/internal/svc"
	"learningplatform/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type LearningplatformLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLearningplatformLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LearningplatformLogic {
	return &LearningplatformLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *LearningplatformLogic) Learningplatform(req *types.Request) (resp *types.Response, err error) {
	// 业务逻辑：把路径参数 name 拼进返回消息
	return &types.Response{
		Message: fmt.Sprintf("Hello, %s!", req.Name),
	}, nil
}
