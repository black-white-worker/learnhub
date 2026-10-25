package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type DeleteDiscussionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDeleteDiscussionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteDiscussionLogic {
	return &DeleteDiscussionLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *DeleteDiscussionLogic) DeleteDiscussion(did int64, cu *class.CtxUser) error {
	d, err := l.svcCtx.ClassStore.GetDiscussion(did)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "帖子不存在"}
	}
	if d.UserId != cu.Id {
		if be := class.RequireTeacher(l.svcCtx, d.ClassId, cu); be != nil {
			return be
		}
	}
	return l.svcCtx.ClassStore.DeleteDiscussion(did)
}
