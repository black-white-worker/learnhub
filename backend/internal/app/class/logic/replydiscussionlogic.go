package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/model"
	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"
)

type ReplyDiscussionLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewReplyDiscussionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReplyDiscussionLogic {
	return &ReplyDiscussionLogic{ctx: ctx, svcCtx: svcCtx}
}

func (l *ReplyDiscussionLogic) ReplyDiscussion(did int64, req *class.ReplyReq, cu *class.CtxUser) error {
	d, err := l.svcCtx.ClassStore.GetDiscussion(did)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "帖子不存在"}
	}
	if _, be := class.RequireClassMember(l.svcCtx, d.ClassId, cu); be != nil {
		return be
	}
	_, err = l.svcCtx.ClassStore.InsertReply(&model.ReplyRow{
		DiscussionId: did, UserId: cu.Id, Content: req.Content,
	})
	return err
}
