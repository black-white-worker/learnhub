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

// Info 获取当前登录用户信息。
//
// 角色与"是否必须改密"以数据库为准：管理员改角色、重置密码之后，
// 前端刷新就会拿到最新状态，不用等 24 小时会话过期。
// 会话只用来确认"你是谁"。
func (l *InfoLogic) Info() (resp *user.InfoResponse, err error) {
	session, ok := l.ctx.Value(user.AuthSessionKey).(model.SessionUser)
	if !ok {
		return nil, errors.New("未登录或登录已过期")
	}

	mustChange := false
	if u := l.svcCtx.UserStore.GetUser(session.Username); u != nil {
		mustChange = u.MustChangePassword
	}

	return &user.InfoResponse{
		Username:           session.Username,
		Nickname:           session.Nickname,
		Role:               session.Role,
		MustChangePassword: mustChange,
	}, nil
}
