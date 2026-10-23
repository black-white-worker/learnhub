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

// 注册限流参数：同一个 IP 一分钟内最多允许注册 registerLimitPerMinute 次
const (
	registerLimitPerMinute = 5
	registerLimitWindowSec = 60
)

// Register 注册业务逻辑（模拟"同事李四"开发的功能）：
// 1. 基础校验：用户名、密码不能为空
// 2. 注册频率限制（Redis INCR 计数，主流防刷做法，key = register:limit:{ip}）
// 3. 调用 UserStore.Add 把新用户写入用户表（用户名重复时 Add 返回错误）
// 4. 成功则返回注册的用户名
func (l *RegisterLogic) Register(req *RegisterRequest) (resp *RegisterResponse, err error) {
	// 1. 基础校验：用户名和密码都不能为空
	if req.Username == "" || req.Password == "" {
		return nil, errors.New("用户名和密码不能为空")
	}

	// 2. 注册频率限制：Redis INCR 自增计数，第一次设置 60 秒过期
	// 超过 5 次直接拒绝，防止脚本恶意刷注册
	ip := getClientIP(l.ctx)
	limitKey := "register:limit:" + ip
	count, err := l.svcCtx.Redis.Incr(limitKey)
	if err != nil {
		return nil, errors.New("服务繁忙，请稍后再试")
	}
	if count == 1 {
		// 第一次计数时设置过期时间，窗口结束后自动清零
		l.svcCtx.Redis.Expire(limitKey, registerLimitWindowSec)
	}
	if count > registerLimitPerMinute {
		return nil, errors.New("注册过于频繁，请稍后再试")
	}

	// 3. 写入用户表（重复用户名由 UserStore.Add 拦截）
	if err := l.svcCtx.UserStore.Add(req.Username, req.Password); err != nil {
		return nil, err
	}

	// 4. 注册成功，返回用户名
	return &RegisterResponse{Username: req.Username}, nil
}

// getClientIP 从 context 取客户端 IP（由 handler 解析请求后放入）。
// 取不到时用 "unknown" 兜底，避免 key 为空。
func getClientIP(ctx context.Context) string {
	if ip, ok := ctx.Value(clientIPKey).(string); ok && ip != "" {
		return ip
	}
	return "unknown"
}
