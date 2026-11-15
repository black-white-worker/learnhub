package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type QuitClassLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewQuitClassLogic(ctx context.Context, svcCtx *svc.ServiceContext) *QuitClassLogic {
	return &QuitClassLogic{ctx: ctx, svcCtx: svcCtx}
}

// QuitClass 学生主动退出班级。
//
// 只删 class_members 这一行：已经交过的作业、考过的试、签到记录都保留。
// 理由有二 ——
//  1. 退班是"我不想再看到这个班"，不是"请删除我的成绩"；
//  2. 老师统计历史成绩时，学期中退班的学生不该凭空从成绩册里消失。
//
// 老师不能走这条路：班里可能没人管了，正确做法是转让或删班（管理员兜底）。
func (l *QuitClassLogic) QuitClass(classId int64, cu *class.CtxUser) error {
	role, be := class.RequireClassMember(l.svcCtx, classId, cu)
	if be != nil {
		return be
	}
	if reason := class.QuitDenyReason(role); reason != "" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: reason}
	}
	return l.svcCtx.ClassStore.RemoveMember(classId, cu.Id)
}
