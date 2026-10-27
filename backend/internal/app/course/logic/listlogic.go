package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// 课程列表缓存 key 与 TTL。
const (
	courseListCacheTTL = 60 * time.Second
)

// courseListFlight 极简 singleflight：同一时刻多个并发请求打同一个 key，
// 只让一个回源 MySQL，其他等结果共享。
var courseListFlight = &flightGroup{}

type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done  chan struct{}
	val   []*course.Course
	total int64
	err   error
}

func (g *flightGroup) Do(key string, fn func() ([]*course.Course, int64, error)) ([]*course.Course, int64, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.total, c.err
	}
	c := &flightCall{done: make(chan struct{})}
	if g.calls == nil {
		g.calls = make(map[string]*flightCall)
	}
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.total, c.err = fn()
	close(c.done)

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.val, c.total, c.err
}

type ListLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListLogic {
	return &ListLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// List 课程分页列表：
// 1. 先读 Redis 缓存（key 含 page+size）
// 2. 未命中用 singleflight 回源 MySQL，结果写回 Redis
// 3. Redis 不可用直接回源
func (l *ListLogic) List(page, size int) (resp *course.ListResponse, err error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	// 缓存 key 带上"版本号"：课程增删改时把版本号 +1，旧 key 自然失效。
	// 这样不用 SCAN 删除（Redis 上 SCAN 很贵），也不会出现"新建的课 60 秒内看不到"。
	cacheKey := fmt.Sprintf("course:list:v%s:p%d:z%d", course.ListCacheVersion(l.svcCtx.Redis), page, size)

	// Redis 可用才走缓存
	if l.svcCtx.Redis != nil {
		if cached, rerr := l.svcCtx.Redis.Get(cacheKey); rerr == nil && cached != "" {
			var box struct {
				Courses []*course.Course `json:"courses"`
				Total   int64            `json:"total"`
			}
			if json.Unmarshal([]byte(cached), &box) == nil {
				return &course.ListResponse{Courses: box.Courses, Total: box.Total, Page: page, PageSize: size}, nil
			}
		}
	}

	courses, total, ferr := courseListFlight.Do(cacheKey, func() ([]*course.Course, int64, error) {
		// 没有注入 MySQL 数据层时（单测 / 本地无库环境）退回内存 store，
		// 保证课程列表在最小依赖下也能工作，而不是 nil 指针 panic。
		if l.svcCtx.CourseStore == nil {
			all := course.Store.List()
			total := int64(len(all))
			start := (page - 1) * size
			if start > len(all) {
				start = len(all)
			}
			end := start + size
			if end > len(all) {
				end = len(all)
			}
			return all[start:end], total, nil
		}
		// 直接走 MySQL 分页，不用内存 store
		rows, t, err := l.svcCtx.CourseStore.ListCoursesPaged(page, size)
		if err != nil {
			return nil, 0, err
		}
		// model.CourseRow -> course.Course
		bucket := l.svcCtx.Config.Minio.Bucket
		out := make([]*course.Course, 0, len(rows))
		for _, r := range rows {
			out = append(out, &course.Course{
				Id: int(r.Id), Name: r.Name, Intro: r.Intro, Teacher: r.Teacher,
				CreatedBy: r.CreatedBy,
				// 私有桶开启时封面也要预签名，否则课程卡片的图会 403。
				// 封面是公开的宣传素材，不参与媒体鉴权。
				CoverUrl:      course.MediaURL(l.svcCtx, course.ObjectNameFromURL(r.CoverUrl, bucket), r.CoverUrl),
				CoverThumbUrl: course.MediaURL(l.svcCtx, course.ObjectNameFromURL(r.CoverThumbUrl, bucket), r.CoverThumbUrl),
			})
		}
		return out, t, nil
	})
	if ferr != nil {
		return nil, ferr
	}

	// 异步写缓存
	if l.svcCtx.Redis != nil {
		box := struct {
			Courses []*course.Course `json:"courses"`
			Total   int64            `json:"total"`
		}{Courses: courses, Total: total}
		if data, mErr := json.Marshal(box); mErr == nil {
			_ = l.svcCtx.Redis.Setex(cacheKey, string(data), int(courseListCacheTTL.Seconds()))
		}
	}
	return &course.ListResponse{Courses: courses, Total: total, Page: page, PageSize: size}, nil
}
