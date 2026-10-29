package logic

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type CreateAssignmentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAssignmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAssignmentLogic {
	return &CreateAssignmentLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateAssignmentLogic) CreateAssignment(classId int64, req *class.CreateAssignmentReq, cu *class.CtxUser) (*class.CreateAssignmentResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	var dl sql.NullTime
	if req.Deadline != "" {
		if t, err := time.Parse("2006-01-02T15:04", req.Deadline); err == nil {
			dl = sql.NullTime{Time: t, Valid: true}
		}
	}
	score := req.TotalScore
	if score <= 0 {
		score = 100
	}
	id, err := l.svcCtx.ClassStore.InsertAssignment(&model.AssignmentRow{
		ClassId: classId, Title: req.Title, Description: req.Description,
		Deadline: dl, TotalScore: score, PublisherId: cu.Id, CourseId: req.CourseId,
	})
	if err != nil {
		return nil, err
	}
	// 问题3：发布作业时给班级其他成员广播通知
	_ = l.svcCtx.ClassStore.BroadcastNotification(
		classId, cu.Id, "assignment",
		"新作业："+req.Title,
		req.Description,
		"/class/"+strconv.FormatInt(classId, 10),
	)
	return &class.CreateAssignmentResp{Id: id}, nil
}
