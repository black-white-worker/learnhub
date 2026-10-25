package logic

import (
	"context"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type StartSigninLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewStartSigninLogic(ctx context.Context, svcCtx *svc.ServiceContext) *StartSigninLogic {
	return &StartSigninLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *StartSigninLogic) StartSignin(classId int64, cu *class.CtxUser) (map[string]interface{}, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	code := class.Rand6Code()
	expiresAt := time.Now().Add(5 * time.Minute)
	id, err := l.svcCtx.ClassStore.CreateSignin(&model.SigninRow{
		ClassId: classId, TeacherId: cu.Id, Code: code, ExpiresAt: expiresAt,
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"id": id, "code": code, "expiresAt": expiresAt}, nil
}
