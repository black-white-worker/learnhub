package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type DiscussionDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewDiscussionDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DiscussionDetailLogic {
	return &DiscussionDetailLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *DiscussionDetailLogic) DiscussionDetail(did int64, cu *class.CtxUser) (*class.DiscussionDetailResp, error) {
	d, err := l.svcCtx.ClassStore.GetDiscussion(did)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "帖子不存在"}
	}
	if _, be := class.RequireClassMember(l.svcCtx, d.ClassId, cu); be != nil {
		return nil, be
	}
	replies, _ := l.svcCtx.ClassStore.ListReplies(did)
	return &class.DiscussionDetailResp{Discussion: d, Replies: replies}, nil
}
