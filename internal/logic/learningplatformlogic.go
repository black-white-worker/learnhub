// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package logic

import (
	"context"

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
	// todo: add your logic here and delete this line

	return
}
