// Package concurrency 提供后端统一的有界并发执行原语。
//
// 为什么要有这个包：
//
//	B 端接口的性能瓶颈通常不是"串行太慢"，而是"为了快就无限开 goroutine"
//	—— 一个班 200 个学生就开 200 个查询，把 MySQL 连接池（go-zero 默认 64）
//	瞬间打满，表现为超时、连接拒绝、P99 抖动。并发必须有上限，并且要跟着
//	请求的 context 一起取消。
//
// 设计约束（刻意保持极小，避免变成通用调度框架）：
//
//  1. limit <= 0 一律按 1 处理，绝不退化成"无界并发"；
//  2. 任一任务返回错误立即取消其余任务（errgroup 语义），由调用方决定是否忽略；
//  3. 任务 panic 转成 error 返回，不让一个坏分支打崩整个进程（后台任务尤其重要）；
//  4. 不隐藏 goroutine 泄漏：ForEach 返回时所有任务都已结束。
//
// 用法：
//
//	err := concurrency.ForEach(ctx, len(ids), 4, func(ctx context.Context, i int) error {
//	    return load(ctx, ids[i])
//	})
package concurrency

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// Task 第 i 个子任务。ctx 会在任一任务出错时被取消。
type Task func(ctx context.Context, i int) error

// PanicError 子任务 panic 时返回的错误，保留原始 panic 值与堆栈，便于定位。
type PanicError struct {
	Value any
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("并发任务 panic: %v", e.Value)
}

// ForEach 以不超过 limit 的并发度执行 n 个任务（i = 0..n-1）。
//
// 返回第一个非 nil 错误（其余任务会被取消）。limit <= 0 时按串行执行。
func ForEach(ctx context.Context, n, limit int, fn Task) error {
	if n <= 0 {
		return nil
	}
	if limit <= 0 {
		limit = 1
	}
	// 并发度不必超过任务数
	if limit > n {
		limit = n
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(limit)
	for i := 0; i < n; i++ {
		i := i
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = &PanicError{Value: r, Stack: debug.Stack()}
				}
			}()
			// SetLimit 已经做了节流；这里再检查一次取消，避免排队任务白跑。
			if cerr := gctx.Err(); cerr != nil {
				return cerr
			}
			return fn(gctx, i)
		})
	}
	return g.Wait()
}

// MaxObserved 测试/观测辅助：记录一组任务里实际达到的最大并发数。
// 业务代码不要用它做限流——限流靠 limit 参数，不靠观测。
type MaxObserved struct {
	cur  int32
	max  int32
	used int32
}

// Enter 标记一个任务开始，返回退出函数（defer 调用）。
func (m *MaxObserved) Enter() func() {
	cur := atomic.AddInt32(&m.cur, 1)
	for {
		old := atomic.LoadInt32(&m.max)
		if cur <= old || atomic.CompareAndSwapInt32(&m.max, old, cur) {
			break
		}
	}
	atomic.AddInt32(&m.used, 1)
	return func() { atomic.AddInt32(&m.cur, -1) }
}

// Max 观测到的最大并发。
func (m *MaxObserved) Max() int { return int(atomic.LoadInt32(&m.max)) }

// Used 进入过任务的总次数。
func (m *MaxObserved) Used() int { return int(atomic.LoadInt32(&m.used)) }
