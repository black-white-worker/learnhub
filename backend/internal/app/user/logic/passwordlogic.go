package logic

// 修改密码：登录用户自己改，必须校验旧密码。
//
// 为什么必须校验旧密码：token 存在 localStorage，一旦被 XSS 或共享设备拿到，
// "只凭 token 就能改密"等于直接把账号交出去。校验旧密码是最低成本的第二因子。

import (
	"context"
	"strings"

	"learningplatform/internal/app/user"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// minPasswordLength 最小密码长度。
// 6 是兼容历史演示账号的下限；上线前建议提到 8 并加复杂度校验。
const minPasswordLength = 6

type PasswordLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewPasswordLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PasswordLogic {
	return &PasswordLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ChangePassword 修改当前登录用户的密码。
func (l *PasswordLogic) ChangePassword(req *user.ChangePasswordRequest) (*user.ChangePasswordResponse, error) {
	session, ok := l.ctx.Value(user.AuthSessionKey).(model.SessionUser)
	if !ok {
		return nil, user.NewAuthError(401, "未登录或登录已过期")
	}

	newPassword := strings.TrimSpace(req.NewPassword)
	if len(newPassword) < minPasswordLength {
		return nil, user.NewAuthError(400, "新密码至少 6 位")
	}
	if newPassword == req.OldPassword {
		return nil, user.NewAuthError(400, "新密码不能与旧密码相同")
	}
	// 先验证旧密码：失败按 401 处理，和"密码错"的语义一致
	if !l.svcCtx.UserStore.Verify(session.Username, req.OldPassword) {
		return nil, user.NewAuthError(401, "原密码不正确")
	}
	if err := l.svcCtx.UserStore.ChangePassword(session.Username, newPassword); err != nil {
		return nil, err
	}

	// 改密后让该用户的其他会话失效的完整实现需要按用户维度维护 token 集合；
	// 当前会话保留（用户刚操作完，不该被踢下线），旧 token 仍受 24h TTL 约束。
	l.Logger.Infof("用户 %s 修改了密码", session.Username)
	return &user.ChangePasswordResponse{Username: session.Username, MustChangePassword: false}, nil
}
