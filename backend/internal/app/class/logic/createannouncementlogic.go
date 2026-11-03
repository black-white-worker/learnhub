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
	// 实时推送：给班级每位在线成员推送一条"新公告"消息。
	// 用独立 goroutine 做 fan-out，HTTP 请求立即返回，不等待推送完成；
	// Push 本身是非阻塞的，离线用户直接丢弃（前端轮询兜底）。
	if members, merr := l.svcCtx.ClassStore.ListMembers(classId); merr == nil {
		payload := []byte(`{"type":"announcement","id":` + strconv.FormatInt(id, 10) + `,"title":` + strconv.Quote(req.Title) + `}`)
		go func(members []*model.ClassMemberRow, p []byte) {
			for _, m := range members {
				l.svcCtx.NotifHub.Push(m.UserId, p)
			}
		}(members, payload)
	}
	class.LogOperation(l.svcCtx, ip, cu.Id, cu.Username, "publish_notice", fmt.Sprintf("class:%d", classId), req.Title)
	return &class.CreateAnnouncementResp{Id: id}, nil
}
