package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type UnbindCourseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUnbindCourseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnbindCourseLogic {
	return &UnbindCourseLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *UnbindCourseLogic) UnbindCourse(classId, courseId int64, cu *class.CtxUser) error {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return be
	}
	return l.svcCtx.ClassStore.UnbindCourse(classId, courseId)
}
