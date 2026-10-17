package user

import (
	"context"
	"errors"
	"fmt"
	"time"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type LoginLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewLoginLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LoginLogic {
	return &LoginLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Login 登录业务逻辑：
// 1. 校验用户名密码（数据来自 model 包的内存用户存储，后续替换为 MySQL）
// 2. 校验通过后生成一个 token 返回给前端
func (l *LoginLogic) Login(req *LoginRequest) (resp *LoginResponse, err error) {
	// 1. 校验用户名密码
	if !l.svcCtx.UserStore.Verify(req.Username, req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	// 2. 生成登录令牌（教学版：简单拼接；后续可换成 JWT 签发）
	token := fmt.Sprintf("token-%s-%d", req.Username, time.Now().Unix())

	// 3. 返回登录结果
	return &LoginResponse{
		Token:    token,
		Username: req.Username,
	}, nil
}
