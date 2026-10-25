package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type BindCourseLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewBindCourseLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BindCourseLogic {
	return &BindCourseLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *BindCourseLogic) BindCourse(classId int64, req *class.BindCourseReq, cu *class.CtxUser) error {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return be
	}
	return l.svcCtx.ClassStore.BindCourse(classId, req.CourseId)
}
