package user

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"

	"learningplatform/internal/model"
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

// loginTokenTTL 登录 token 在 Redis 中的有效期（秒），24 小时
const loginTokenTTL = 24 * 3600

// Login 登录业务逻辑（主流 Redis 会话方案）：
// 1. 校验用户名密码（数据来自 MySQL 用户表）
// 2. 校验通过后生成随机 token（32 字节十六进制，不可预测）
// 3. 把会话信息（用户名 + 昵称）以 JSON 写入 Redis，TTL 24 小时
// 4. 返回 token 给前端，后续请求带 Authorization: Bearer {token}
func (l *LoginLogic) Login(req *LoginRequest) (resp *LoginResponse, err error) {
	// 1. 校验用户名密码
	if !l.svcCtx.UserStore.Verify(req.Username, req.Password) {
		return nil, errors.New("用户名或密码错误")
	}

	// 2. 生成随机 token（crypto/rand，比拼接时间戳更安全，主流的做法）
	token, err := generateLoginToken()
	if err != nil {
		return nil, errors.New("令牌生成失败")
	}

	// 3. 查用户信息（取昵称，构造会话）
	u := l.svcCtx.UserStore.GetUser(req.Username)
	if u == nil {
		return nil, errors.New("用户信息读取失败")
	}

	// 4. 会话信息序列化为 JSON 写入 Redis
	// key = login:token:{token}，value = {"username":..., "nickname":...}
	session := model.SessionUser{Username: u.Username, Nickname: u.Nickname, Role: u.Role}
	data, err := json.Marshal(session)
	if err != nil {
		return nil, errors.New("会话序列化失败")
	}
	if err := l.svcCtx.Redis.Setex(sessionKeyPrefix+token, string(data), loginTokenTTL); err != nil {
		return nil, errors.New("登录状态写入失败")
	}

	// 5. 返回登录结果（带上角色，前端据此控制"创建班级"等入口）
	return &LoginResponse{
		Token:    token,
		Username: req.Username,
		Role:     u.Role,
	}, nil
}

// generateLoginToken 生成 32 字节的随机 token（十六进制 64 位）。
// 使用 crypto/rand 安全随机源，避免 token 被猜测。
func generateLoginToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
