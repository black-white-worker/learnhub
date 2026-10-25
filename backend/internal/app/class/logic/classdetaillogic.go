package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ClassDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewClassDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClassDetailLogic {
	return &ClassDetailLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ClassDetailLogic) ClassDetail(classId int64, cu *class.CtxUser) (*class.ClassDetailResp, error) {
	role, be := class.RequireClassMember(l.svcCtx, classId, cu)
	if be != nil {
		return nil, be
	}
	c, err := l.svcCtx.ClassStore.GetClass(classId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "班级不存在"}
	}
	members, _ := l.svcCtx.ClassStore.ListMembers(classId)
	courseIds, _ := l.svcCtx.ClassStore.ListClassCourseIds(classId)
	inviteCode := ""
	if invs, _ := l.svcCtx.ClassStore.ListInvitations(classId); len(invs) > 0 {
		inviteCode = invs[len(invs)-1].Code
	}
	return &class.ClassDetailResp{
		Class: c, Members: members, CourseIds: courseIds, InviteCode: inviteCode,
		MyRole: role,
	}, nil
}
