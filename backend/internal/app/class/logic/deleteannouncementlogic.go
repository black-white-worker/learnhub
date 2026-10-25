package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type DeleteAnnouncementLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteAnnouncementLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteAnnouncementLogic {
	return &DeleteAnnouncementLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteAnnouncementLogic) DeleteAnnouncement(aid int64, cu *class.CtxUser) error {
	a, err := l.svcCtx.ClassStore.GetAnnouncement(aid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "公告不存在"}
	}
	if a.PublisherId != cu.Id {
		if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
			return be
		}
	}
	return l.svcCtx.ClassStore.DeleteAnnouncement(aid)
}
