package logic

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"learningplatform/internal/app/course"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// 课程列表缓存 key 与 TTL。
// 课程是低频写、高频读的场景：首页每次刷新都打这个接口，
// 加 Redis 缓存 + singleflight 防缓存击穿。
const (
	courseListCacheKey = "course:list:v1"
	courseListCacheTTL = 60 * time.Second
)

// courseListFlight 极简 singleflight：同一时刻多个并发请求打同一个 key，
// 只让一个回源（调 course.Store.List()），其他等结果共享。
// 避免缓存失效瞬间大量请求同时穿透到 MySQL。
var courseListFlight = &flightGroup{}

type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	val  []*course.Course
	err  error
}

func (g *flightGroup) Do(key string, fn func() ([]*course.Course, error)) ([]*course.Course, error) {
	g.mu.Lock()
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		<-c.done
		return c.val, c.err
	}
	c := &flightCall{done: make(chan struct{})}
	if g.calls == nil {
		g.calls = make(map[string]*flightCall)
	}
	g.calls[key] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	close(c.done)

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.val, c.err
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

// List 课程列表业务逻辑：
// 1. 先读 Redis 缓存
// 2. 未命中用 singleflight 回源到 course.Store.List()，结果写回 Redis
// 3. Redis 不可用时直接回源（降级，不能因为缓存挂了接口就挂）
func (l *ListLogic) List() (resp *course.ListResponse, err error) {
	// Redis 可用才走缓存
	if l.svcCtx.Redis != nil {
		if cached, rerr := l.svcCtx.Redis.Get(courseListCacheKey); rerr == nil && cached != "" {
			var courses []*course.Course
			if json.Unmarshal([]byte(cached), &courses) == nil {
				return &course.ListResponse{Courses: courses}, nil
			}
			// 反序列化失败不致命，回源重建缓存
		}
	}

	courses, ferr := courseListFlight.Do(courseListCacheKey, func() ([]*course.Course, error) {
		return course.Store.List(), nil
	})
	if ferr != nil {
		return nil, ferr
	}

	// 异步写缓存，失败只记日志（不阻塞响应）
	if l.svcCtx.Redis != nil {
		if data, mErr := json.Marshal(courses); mErr == nil {
			_ = l.svcCtx.Redis.Setex(courseListCacheKey, string(data), int(courseListCacheTTL.Seconds()))
		}
	}
	return &course.ListResponse{Courses: courses}, nil
}
