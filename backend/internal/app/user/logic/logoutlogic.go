package logic

import (
	"context"
	"errors"

	"learningplatform/internal/model"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type LogoutLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLogoutLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LogoutLogic {
	return &LogoutLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Logout 登出业务逻辑（主流 Redis 会话方案）：
// 1. token 已经过鉴权中间件校验，这里直接从 context 取
// 2. 删除 Redis 里的会话 key（login:token:{token}），登录态立即失效
// 3. 返回被登出的用户名
func (l *LogoutLogic) Logout(token string) (resp *user.LogoutResponse, err error) {
	// 1. 会话信息从 context 取（由鉴权中间件放入）
	session, _ := l.ctx.Value(user.AuthSessionKey).(model.SessionUser)

	// 2. 删除 Redis 会话，登出即失效，无需等 TTL 过期
	if _, err := l.svcCtx.Redis.Del(user.SessionKeyPrefix + token); err != nil {
		return nil, errors.New("登出失败，请稍后再试")
	}

	// 3. 返回登出结果
	return &user.LogoutResponse{Username: session.Username}, nil
}
