package logic

// 公告列表。
//
// 这一页有两个曾经被当成"并发问题"的东西，实际上是查询问题：
//
//	1. N+1：每条公告各查一次 announcement_reads（旧实现甚至把整份已读名单拉回来遍历）；
//	2. 为了掩盖 N+1，用 errgroup 开 8 个 goroutine 并发查——延迟好看了一点，
//	   但总查询数、总传输字节、连接池占用都没变，P99 反而更抖。
//
// 现在的做法：一条 GROUP BY 把整页公告的"已读数 + 我是否已读"算完，
// 往返固定 2 次（列表 1 次 + 已读聚合 1 次），与页大小无关。
// 参考实现：Canvas LMS 的 AnnouncementsController#index 用 preload 聚合已读状态；
// 规约见《阿里巴巴 Java 开发手册》"禁止在循环体内查询数据库"。

import (
	"context"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListAnnouncementsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAnnouncementsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAnnouncementsLogic {
	return &ListAnnouncementsLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListAnnouncements 公告分页列表（已读状态批量聚合）。
func (l *ListAnnouncementsLogic) ListAnnouncements(classId int, page, size int, cu *class.CtxUser) (interface{}, error) {
	classId64 := int64(classId)
	if _, be := class.RequireClassMember(l.svcCtx, classId64, cu); be != nil {
		return nil, be
	}
	list, total, err := l.svcCtx.ClassStore.ListAnnouncementsPaged(classId64, page, size)
	if err != nil {
		return nil, err
	}
	out := l.withReadState(list, cu.Id)
	return map[string]interface{}{
		"list":     out,
		"total":    total,
		"page":     page,
		"pageSize": size,
	}, nil
}

// ListAnnouncementsAll 兼容旧签名（不分页）。
func (l *ListAnnouncementsLogic) ListAnnouncementsAll(classId int64, cu *class.CtxUser) ([]*class.AnnouncementWithRead, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	list, err := l.svcCtx.ClassStore.ListAnnouncements(classId)
	if err != nil {
		return nil, err
	}
	return l.withReadState(list, cu.Id), nil
}

// withReadState 给一页公告补上已读状态，固定 1 次 SQL。
// 聚合失败时降级为"未读/0 人"：已读标记是辅助信息，不应当把公告列表整体打成 500。
func (l *ListAnnouncementsLogic) withReadState(list []*model.AnnouncementRow, userId int64) []*class.AnnouncementWithRead {
	ids := make([]int64, 0, len(list))
	for _, a := range list {
		ids = append(ids, a.Id)
	}
	stats, err := l.svcCtx.ClassStore.AnnouncementReadStats(userId, ids)
	if err != nil {
		logx.WithContext(l.ctx).Errorf("查询公告已读状态失败（降级为未读）: %v", err)
		stats = map[int64]model.AnnouncementReadStat{}
	}
	out := make([]*class.AnnouncementWithRead, 0, len(list))
	for _, a := range list {
		st := stats[a.Id]
		out = append(out, &class.AnnouncementWithRead{
			AnnouncementRow: a,
			ReadByMe:        st.ReadByMe,
			ReadCount:       st.ReadCount,
		})
	}
	return out
}
