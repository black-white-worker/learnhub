package logic

import (
	"context"
	"strconv"

	"learningplatform/internal/model"
	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type CreateDiscussionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateDiscussionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateDiscussionLogic {
	return &CreateDiscussionLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateDiscussionLogic) CreateDiscussion(classId int64, req *class.CreateDiscussionReq, cu *class.CtxUser) (*class.CreateDiscussionResp, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	id, err := l.svcCtx.ClassStore.InsertDiscussion(&model.DiscussionRow{
		ClassId: classId, UserId: cu.Id, Title: req.Title, Content: req.Content,
	})
	if err != nil {
		return nil, err
	}
	// 问题3：新讨论给班级其他成员广播通知
	_ = l.svcCtx.ClassStore.BroadcastNotification(
		classId, cu.Id, "discussion",
		"新讨论："+req.Title,
		req.Content,
		"/class/"+strconv.FormatInt(classId, 10),
	)
	return &class.CreateDiscussionResp{Id: id}, nil
}
