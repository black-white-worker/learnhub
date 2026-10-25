package logic

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ListAnnouncementsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAnnouncementsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAnnouncementsLogic {
	return &ListAnnouncementsLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ListAnnouncementsLogic) ListAnnouncements(classId int64, cu *class.CtxUser) ([]*class.AnnouncementWithRead, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	list, err := l.svcCtx.ClassStore.ListAnnouncements(classId)
	if err != nil {
		return nil, err
	}
	out := make([]*class.AnnouncementWithRead, 0, len(list))
	for _, a := range list {
		readIds, _ := l.svcCtx.ClassStore.ListReadUserIds(a.Id)
		readByMe := false
		for _, uid := range readIds {
			if uid == cu.Id {
				readByMe = true
				break
			}
		}
		out = append(out, &class.AnnouncementWithRead{AnnouncementRow: a, ReadByMe: readByMe, ReadCount: len(readIds)})
	}
	return out, nil
}
