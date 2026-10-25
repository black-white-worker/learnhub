package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type MarkAnnouncementReadLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewMarkAnnouncementReadLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MarkAnnouncementReadLogic {
	return &MarkAnnouncementReadLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *MarkAnnouncementReadLogic) MarkAnnouncementRead(aid int64, cu *class.CtxUser) error {
	return l.svcCtx.ClassStore.MarkAnnouncementRead(aid, cu.Id)
}
