package concurrency

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// 用例：并发度永远不会超过 limit，且每个下标恰好执行一次。
func TestForEach_Bounded(t *testing.T) {
	const n, limit = 60, 4
	var seen [n]int32
	var obs MaxObserved

	err := ForEach(context.Background(), n, limit, func(ctx context.Context, i int) error {
		defer obs.Enter()()
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&seen[i], 1)
		return nil
	})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if got := obs.Max(); got > limit {
		t.Fatalf("并发度超过上限: 观测到 %d，上限 %d", got, limit)
	}
	if got := obs.Used(); got != n {
		t.Fatalf("任务执行次数应为 %d，实际 %d", n, got)
	}
	for i, c := range seen {
		if c != 1 {
			t.Fatalf("下标 %d 执行了 %d 次", i, c)
		}
	}
}

// 用例：limit <= 0 必须退化成串行，而不是无界并发。
func TestForEach_NonPositiveLimitIsSerial(t *testing.T) {
	var obs MaxObserved
	_ = ForEach(context.Background(), 10, 0, func(ctx context.Context, i int) error {
		defer obs.Enter()()
		time.Sleep(time.Millisecond)
		return nil
	})
	if got := obs.Max(); got != 1 {
		t.Fatalf("limit=0 应串行执行，实际最大并发 %d", got)
	}
}

// 用例：错误会向上返回，并且后续任务不再执行（串行场景可确定）。
func TestForEach_ErrorStopsRemaining(t *testing.T) {
	var used int32
	boom := errors.New("boom")
	err := ForEach(context.Background(), 10, 1, func(ctx context.Context, i int) error {
		atomic.AddInt32(&used, 1)
		if i == 2 {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("应返回原始错误，实际 %v", err)
	}
	if got := atomic.LoadInt32(&used); got != 3 {
		t.Fatalf("出错后不应继续执行，实际执行 %d 个任务", got)
	}
}

// 用例：任务 panic 不能打崩进程，要转成 *PanicError 返回。
func TestForEach_PanicBecomesError(t *testing.T) {
	err := ForEach(context.Background(), 3, 2, func(ctx context.Context, i int) error {
		if i == 1 {
			panic("测试 panic")
		}
		return nil
	})
	var pe *PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("应返回 *PanicError，实际 %T %v", err, err)
	}
	if len(pe.Stack) == 0 {
		t.Fatal("PanicError 应保留堆栈")
	}
}

// 用例：n <= 0 直接返回，不调用任何任务。
func TestForEach_Empty(t *testing.T) {
	called := false
	if err := ForEach(context.Background(), 0, 4, func(ctx context.Context, i int) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("空任务不应报错: %v", err)
	}
	if called {
		t.Fatal("空任务不应被执行")
	}
}

// 用例：父 context 取消后，排队中的任务返回取消错误而不是继续跑。
func TestForEach_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := ForEach(ctx, 100, 2, func(ctx context.Context, i int) error {
		if i == 0 {
			cancel()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
			return nil
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("期望 context.Canceled，实际 %v", err)
	}
}
