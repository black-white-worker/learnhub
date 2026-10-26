package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type EndSigninLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewEndSigninLogic(ctx context.Context, svcCtx *svc.ServiceContext) *EndSigninLogic {
	return &EndSigninLogic{ctx: ctx, svcCtx: svcCtx}
}

// EndSignin 老师手动结束当前进行中的签到。
// 直接把 expires_at 改成 now()，GetActiveSignin 就查不到了。
func (l *EndSigninLogic) EndSignin(classId int64, cu *class.CtxUser) error {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return be
	}
	active, err := l.svcCtx.ClassStore.GetActiveSignin(classId)
	if err != nil || active == nil {
		return &class.BizError{Code: 404, Msg: "当前没有进行中的签到"}
	}
	return l.svcCtx.ClassStore.EndSignin(active.Id)
}
