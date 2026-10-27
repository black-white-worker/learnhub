package logic

import (
	"context"
	"fmt"
	"strconv"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type CreateAnnouncementLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateAnnouncementLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateAnnouncementLogic {
	return &CreateAnnouncementLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *CreateAnnouncementLogic) CreateAnnouncement(classId int64, req *class.CreateAnnouncementReq, cu *class.CtxUser, ip string) (*class.CreateAnnouncementResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	id, err := l.svcCtx.ClassStore.InsertAnnouncement(&model.AnnouncementRow{
		ClassId: classId, Title: req.Title, Content: req.Content,
		PublisherId: cu.Id, IsTop: req.IsTop,
	})
	if err != nil {
		return nil, err
	}
	// 问题3：发布公告时给班级其他成员广播通知
	_ = l.svcCtx.ClassStore.BroadcastNotification(
		classId, cu.Id, "announcement",
		"新公告："+req.Title,
		req.Content,
		"/class/"+strconv.FormatInt(classId, 10),
	)
	class.LogOperation(l.svcCtx, ip, cu.Id, cu.Username, "publish_notice", fmt.Sprintf("class:%d", classId), req.Title)
	return &class.CreateAnnouncementResp{Id: id}, nil
}
