package user

import (
	"context"
	"errors"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type RegisterLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewRegisterLogic(ctx context.Context, svcCtx *svc.ServiceContext) *RegisterLogic {
	return &RegisterLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Register 注册业务逻辑（模拟"同事李四"开发的功能）：
// 1. 基础校验：用户名、密码不能为空
// 2. 调用 UserStore.Add 把新用户写入用户表（用户名重复时 Add 返回错误）
// 3. 成功则返回注册的用户名
func (l *RegisterLogic) Register(req *RegisterRequest) (resp *RegisterResponse, err error) {
	// 1. 基础校验：用户名和密码都不能为空
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	// 2. 写入用户表（重复用户名由 UserStore.Add 拦截）
	if err := l.svcCtx.UserStore.Add(req.Username, req.Password); err != nil {
		return nil, err
	}

	// 3. 注册成功，返回用户名
	return &RegisterResponse{Username: req.Username}, nil
}
