package logic

import (
	"context"
	"sync"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"golang.org/x/sync/errgroup"
)

type ListAnnouncementsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListAnnouncementsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListAnnouncementsLogic {
	return &ListAnnouncementsLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListAnnouncements 公告分页列表。
// 并发点：每条公告的已读用户列表原来是 for 循环串行 N 次 SQL，
// 这里用 errgroup + 带缓冲 semaphore 并发查，DB 连接池足够时延迟从 N*rt 降到 ~rt。
func (l *ListAnnouncementsLogic) ListAnnouncements(classId int, page, size int, cu *class.CtxUser) (interface{}, error) {
	classId64 := int64(classId)
	if _, be := class.RequireClassMember(l.svcCtx, classId64, cu); be != nil {
		return nil, be
	}
	list, total, err := l.svcCtx.ClassStore.ListAnnouncementsPaged(classId64, page, size)
	if err != nil {
		return nil, err
	}

	// 并发查每条公告的已读状态
	type readResult struct {
		idx      int
		readByMe bool
		readCnt  int
	}
	results := make([]readResult, len(list))
	g, gctx := errgroup.WithContext(l.ctx)
	// 限制并发度最多 8，防止一条公告开一个 goroutine 把 MySQL 连接池打满
	sem := make(chan struct{}, 8)
	for i, a := range list {
		i, a := i, a // 闭包捕获
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			select {
			case <-gctx.Done():
				return gctx.Err()
			default:
			}
			readIds, _ := l.svcCtx.ClassStore.ListReadUserIds(a.Id)
			readByMe := false
			for _, uid := range readIds {
				if uid == cu.Id {
					readByMe = true
					break
				}
			}
			results[i] = readResult{idx: i, readByMe: readByMe, readCnt: len(readIds)}
			return nil
		})
	}
	_ = g.Wait() // 单个失败不阻塞主流程，已读状态降级为 false

	out := make([]*class.AnnouncementWithRead, 0, len(list))
	for i, a := range list {
		out = append(out, &class.AnnouncementWithRead{
			AnnouncementRow: a,
			ReadByMe:        results[i].readByMe,
			ReadCount:       results[i].readCnt,
		})
	}
	return map[string]interface{}{
		"list":     out,
		"total":    total,
		"page":     page,
		"pageSize": size,
	}, nil
}

// 兼容旧签名（不分页），保留给旧调用方
func (l *ListAnnouncementsLogic) ListAnnouncementsAll(classId int64, cu *class.CtxUser) ([]*class.AnnouncementWithRead, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	list, err := l.svcCtx.ClassStore.ListAnnouncements(classId)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	out := make([]*class.AnnouncementWithRead, len(list))
	g, _ := errgroup.WithContext(l.ctx)
	sem := make(chan struct{}, 8)
	for i, a := range list {
		i, a := i, a
		g.Go(func() error {
			sem <- struct{}{}
			defer func() { <-sem }()
			readIds, _ := l.svcCtx.ClassStore.ListReadUserIds(a.Id)
			readByMe := false
			for _, uid := range readIds {
				if uid == cu.Id {
					readByMe = true
					break
				}
			}
			mu.Lock()
			out[i] = &class.AnnouncementWithRead{AnnouncementRow: a, ReadByMe: readByMe, ReadCount: len(readIds)}
			mu.Unlock()
			return nil
		})
	}
	_ = g.Wait()
	return out, nil
}

// 防 unused import
var _ = model.AnnouncementRow{}
