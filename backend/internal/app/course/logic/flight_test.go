package logic

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"learningplatform/internal/app/course"
)

// 并发 10 个请求打同一个 key，fn 应该只被调用 1 次（singleflight 核心保证）。
func TestFlightGroup_ConcurrentSingleFlight(t *testing.T) {
	// 每个测试用独立 key，避免和其他测试互相干扰
	key := "test-singleflight-" + time.Now().Format("150405.000000")

	var callCount int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = courseListFlight.Do(key, func() ([]*course.Course, error) {
				atomic.AddInt32(&callCount, 1)
				// 睡 10ms，确保其他 9 个 goroutine 在这期间都在等
				time.Sleep(10 * time.Millisecond)
				return nil, nil
			})
		}()
	}
	wg.Wait()

	if callCount != 1 {
		t.Errorf("singleflight 期望 fn 只被调用 1 次，实际 %d 次", callCount)
	}
}

// 不同 key 应该各自调用 fn（不互相阻塞）。
func TestFlightGroup_DifferentKeys(t *testing.T) {
	var callCount int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		key := "test-sep-" + time.Now().Format("150405.000000000") + string(rune('A'+i))
		go func(k string) {
			defer wg.Done()
			_, _ = courseListFlight.Do(k, func() ([]*course.Course, error) {
				atomic.AddInt32(&callCount, 1)
				return nil, nil
			})
		}(key)
	}
	wg.Wait()

	if callCount != 5 {
		t.Errorf("不同 key 期望各自调用 fn（5 次），实际 %d", callCount)
	}
}
