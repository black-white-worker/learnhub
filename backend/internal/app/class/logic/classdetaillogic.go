package logic

import (
	"context"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/concurrency"
	"learningplatform/internal/svc"
)

type ClassDetailLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewClassDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ClassDetailLogic {
	return &ClassDetailLogic{ctx: ctx, svcCtx: svcCtx}
}

// classDetailFanout 班级详情一次要取的四段数据，并发度封顶 4。
//
// 这四段查询互不依赖，串行执行时页面延迟 = 4 × RTT；有界并发后 ≈ 1 × RTT。
// 之所以不是"每条数据开一个 goroutine"：RTT 才是收益来源，
// 无界并发只会把连接池（go-zero 默认 64）打满，制造新的排队。
const classDetailFanout = 4

func (l *ClassDetailLogic) ClassDetail(classId int64, cu *class.CtxUser) (*class.ClassDetailResp, error) {
	role, be := class.RequireClassMember(l.svcCtx, classId, cu)
	if be != nil {
		return nil, be
	}

	var (
		cls         *model.ClassRow
		members     []*model.ClassMemberRow
		courseIds   []int64
		invitations []*model.InvitationRow
	)
	err := concurrency.ForEach(l.ctx, classDetailFanout, classDetailFanout, func(ctx context.Context, i int) error {
		switch i {
		case 0:
			var e error
			cls, e = l.svcCtx.ClassStore.GetClass(classId)
			return e
		case 1:
			// 成员/课程/邀请码属于"附加信息"：查失败时降级为空，
			// 与改造前一致（不因为某个列表挂了就把详情页打成 500）。
			members, _ = l.svcCtx.ClassStore.ListMembers(classId)
		case 2:
			courseIds, _ = l.svcCtx.ClassStore.ListClassCourseIds(classId)
		case 3:
			invitations, _ = l.svcCtx.ClassStore.ListInvitations(classId)
		}
		return nil
	})
	if err != nil || cls == nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "班级不存在"}
	}

	inviteCode := ""
	if len(invitations) > 0 {
		inviteCode = invitations[len(invitations)-1].Code
	}
	return &class.ClassDetailResp{
		Class: cls, Members: members, CourseIds: courseIds, InviteCode: inviteCode,
		MyRole: role,
	}, nil
}
