package logic

import (
	"context"
	"errors"

	"learningplatform/internal/app/user"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type InfoLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewInfoLogic(ctx context.Context, svcCtx *svc.ServiceContext) *InfoLogic {
	return &InfoLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Info 获取当前登录用户信息（演示鉴权中间件 + Redis 会话的用法）：
// 会话信息由鉴权中间件从 Redis 读出并放入 context，无需再查数据库。
func (l *InfoLogic) Info() (resp *user.InfoResponse, err error) {
	// 1. 从 context 取会话信息（鉴权中间件已保证登录态有效）
	session, ok := l.ctx.Value(user.AuthSessionKey).(model.SessionUser)
	if !ok {
		return nil, errors.New("未登录或登录已过期")
	}

	// 2. 返回当前登录用户（角色从会话里取，前端据此控制入口）
	return &user.InfoResponse{
		Username: session.Username,
		Nickname: session.Nickname,
		Role:     session.Role,
	}, nil
}
