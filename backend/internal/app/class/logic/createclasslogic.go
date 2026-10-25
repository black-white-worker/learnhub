package logic

import (
	"context"
	"fmt"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type CreateClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateClassLogic {
	return &CreateClassLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateClassLogic) CreateClass(req *class.CreateClassReq, cu *class.CtxUser, ip string) (*class.CreateClassResp, error) {
	if cu.Role != "teacher" && cu.Role != "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "只有老师或管理员才能创建班级"}
	}
	if req.Name == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "班级名不能为空"}
	}
	id, err := l.svcCtx.ClassStore.InsertClass(req.Name, req.Grade, req.Major, cu.Username, cu.Id)
	if err != nil {
		return nil, err
	}
	if err := l.svcCtx.ClassStore.AddMember(id, cu.Id, "teacher"); err != nil {
		_ = l.svcCtx.ClassStore.DeleteClass(id)
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "加入班级失败：" + err.Error()}
	}
	code := class.Rand6Code()
	if err := l.svcCtx.ClassStore.CreateInvitation(id, code); err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "邀请码生成失败：" + err.Error()}
	}
	class.LogOperation(l.svcCtx, ip, cu.Id, cu.Username, "create_class", fmt.Sprintf("class:%d", id), req.Name)
	return &class.CreateClassResp{Id: id, InviteCode: code}, nil
}
