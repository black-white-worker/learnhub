package course

// 视频后处理 worker 的状态机测试。
//
// 这里不碰 MySQL 也不碰 ffmpeg：videoQueue / faststartRun 两个注入点
// 就是为了让"认领失败不写回、失败够次数才置 failed、进程中断退回 pending"
// 这几条规则可验证 —— 它们出错时的表现是线上视频永远转不了码或反复重转烧 CPU，
// 光靠人肉点上传看不出来。

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/platform/concurrency"
	"learningplatform/internal/svc"
)

// fakeVideoQueue 内存版后处理队列，记录全部写回。
type fakeVideoQueue struct {
	mu       sync.Mutex
	pending  []*model.VideoRow
	claimOK  map[int64]bool // false = 模拟"被别的 worker 抢走"
	listErr  error
	finishes []finishCall
	resets   []time.Duration
}

type finishCall struct {
	id      int64
	status  string
	newUrl  string
	procErr string
}

func newFakeQueue(rows ...*model.VideoRow) *fakeVideoQueue {
	q := &fakeVideoQueue{claimOK: map[int64]bool{}}
	for _, r := range rows {
		q.pending = append(q.pending, r)
		q.claimOK[r.Id] = true
	}
	return q
}

func (q *fakeVideoQueue) ListPendingVideos(limit int) ([]*model.VideoRow, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.listErr != nil {
		return nil, q.listErr
	}
	if limit <= len(q.pending) {
		return append([]*model.VideoRow(nil), q.pending[:limit]...), nil
	}
	return append([]*model.VideoRow(nil), q.pending...), nil
}

func (q *fakeVideoQueue) ClaimPendingVideo(id int64) (bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claimOK[id], nil
}

func (q *fakeVideoQueue) FinishVideoProcess(id int64, status, newUrl, procErr string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.finishes = append(q.finishes, finishCall{id, status, newUrl, procErr})
	return nil
}

func (q *fakeVideoQueue) ResetStaleProcessingVideos(after time.Duration) (int64, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.resets = append(q.resets, after)
	return 0, nil
}

func (q *fakeVideoQueue) written(id int64) []finishCall {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []finishCall
	for _, f := range q.finishes {
		if f.id == id {
			out = append(out, f)
		}
	}
	return out
}

// withFaststart 临时替换转码实现，测试结束自动还原。
func withFaststart(t *testing.T, fn func(context.Context, *svc.ServiceContext, *Video) (string, error)) {
	t.Helper()
	old := faststartRun
	faststartRun = fn
	t.Cleanup(func() { faststartRun = old })
}

func pendingRow(id int64, attempts int) *model.VideoRow {
	return &model.VideoRow{
		Id: id, CourseId: 1, Name: "a.mp4", Url: "http://minio/a.mp4",
		ObjectName: "course/1/videos/a.mp4", ProcessStatus: model.VideoProcessPending,
		ProcessAttempts: attempts,
	}
}

func TestVideoProcessor_SuccessWritesDone(t *testing.T) {
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		return "http://minio/faststart/a.mp4", nil
	})
	q := newFakeQueue(pendingRow(11, 0))

	if n := runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q); n != 1 {
		t.Fatalf("应处理成功 1 个，实际 %d", n)
	}
	got := q.written(11)
	if len(got) != 1 {
		t.Fatalf("只应写回一次状态，实际 %d 次", len(got))
	}
	if got[0].status != model.VideoProcessDone || got[0].newUrl != "http://minio/faststart/a.mp4" {
		t.Fatalf("写回内容不对: %+v", got[0])
	}
}

// 认领失败（别的实例抢先）时绝不能写回状态，否则会把别人的 processing 覆盖掉。
func TestVideoProcessor_LostClaimWritesNothing(t *testing.T) {
	called := false
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		called = true
		return "", nil
	})
	q := newFakeQueue(pendingRow(12, 0))
	q.claimOK[12] = false

	if n := runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q); n != 0 {
		t.Fatalf("没抢到认领不该算成功，实际 %d", n)
	}
	if called || len(q.finishes) != 0 {
		t.Fatalf("未认领的行不应转码或写回: called=%v finishes=%+v", called, q.finishes)
	}
}

// 失败重试：前两次退回 pending，第三次（达到上限）才置 failed。
func TestVideoProcessor_FailureEscalatesToFailed(t *testing.T) {
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		return "", errors.New("ffmpeg exit 1")
	})

	for attempts, want := range map[int]string{
		0: model.VideoProcessPending,
		1: model.VideoProcessPending,
		2: model.VideoProcessFailed,
	} {
		q := newFakeQueue(pendingRow(13, attempts))
		runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q)
		got := q.written(13)
		if len(got) != 1 {
			t.Fatalf("尝试次数 %d：应写回一次，实际 %+v", attempts, got)
		}
		if got[0].status != want {
			t.Fatalf("已有尝试 %d 次后应变成 %s，实际 %s", attempts, want, got[0].status)
		}
		if want == model.VideoProcessFailed && got[0].procErr == "" {
			t.Fatalf("failed 必须带上失败原因，前端重试提示要用")
		}
	}
}

// 关停/超时打断不是视频的错：退回 pending，不写 failed。
func TestVideoProcessor_CancelledContextRequeuesNotFails(t *testing.T) {
	withFaststart(t, func(ctx context.Context, _ *svc.ServiceContext, _ *Video) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	q := newFakeQueue(pendingRow(14, 0))

	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	runVideoProcessRound(ctx, &svc.ServiceContext{}, q)

	got := q.written(14)
	if len(got) != 1 {
		t.Fatalf("应恰好写回一次，实际 %+v", got)
	}
	if got[0].status != model.VideoProcessPending {
		t.Fatalf("被中断的任务应退回 pending，实际 %s（%s）", got[0].status, got[0].procErr)
	}
}

// 没有源对象名的历史脏数据：直接 failed 并说明原因，不占着队列反复跑。
func TestVideoProcessor_MissingObjectNameFailsFast(t *testing.T) {
	called := false
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		called = true
		return "", nil
	})
	row := pendingRow(15, 0)
	row.ObjectName = ""
	q := newFakeQueue(row)

	runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q)
	if called {
		t.Fatal("缺源对象名不该真的去转码")
	}
	got := q.written(15)
	if len(got) != 1 || got[0].status != model.VideoProcessFailed {
		t.Fatalf("应一次性置 failed，实际 %+v", got)
	}
}

// 转码实现 panic 不能带崩 worker，也不能留下永远 processing 的行。
func TestVideoProcessor_PanicMarkedFailedNotStuck(t *testing.T) {
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		panic("boom")
	})
	q := newFakeQueue(pendingRow(16, videoProcessMaxAttempts-1))

	runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q)
	got := q.written(16)
	if len(got) != 1 || got[0].status != model.VideoProcessFailed {
		t.Fatalf("panic 且已达重试上限时应置 failed，实际 %+v", got)
	}
}

// 并发上限是这套改造对线上机器最重要的承诺：转码不许抢满 CPU。
func TestVideoProcessor_ConcurrencyBounded(t *testing.T) {
	var obs concurrency.MaxObserved
	withFaststart(t, func(context.Context, *svc.ServiceContext, *Video) (string, error) {
		leave := obs.Enter()
		defer leave()
		time.Sleep(10 * time.Millisecond)
		return "u", nil
	})

	rows := make([]*model.VideoRow, 0, videoProcessBatch)
	for i := 0; i < videoProcessBatch; i++ {
		rows = append(rows, pendingRow(int64(100+i), 0))
	}
	q := newFakeQueue(rows...)

	if n := runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q); n != len(rows) {
		t.Fatalf("应全部处理成功，实际 %d/%d", n, len(rows))
	}
	if obs.Used() != len(rows) {
		t.Fatalf("漏跑了任务：%d/%d", obs.Used(), len(rows))
	}
	if obs.Max() > videoProcessConcurrency {
		t.Fatalf("并发超过上限 %d，实际 %d", videoProcessConcurrency, obs.Max())
	}
}

func TestVideoProcessor_ListErrorAbortsRound(t *testing.T) {
	q := newFakeQueue()
	q.listErr = errors.New("db down")
	if n := runVideoProcessRound(context.Background(), &svc.ServiceContext{}, q); n != 0 {
		t.Fatalf("捞队列失败应直接结束本轮，实际处理 %d", n)
	}
}

func TestVideoProcessor_WakeIsNonBlocking(t *testing.T) {
	// 连续两次唤醒：第二次通道已满，必须立刻返回而不是等 worker 消费。
	WakeVideoProcessor()
	done := make(chan struct{})
	go func() { WakeVideoProcessor(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("WakeVideoProcessor 在通道满时阻塞了上传请求")
	}
	// 收掉信号，避免影响同包其他测试
	select {
	case <-videoProcessWake:
	default:
	}
}

func TestVideoProcessor_StartWithoutDepsExitsImmediately(t *testing.T) {
	// 没接 DB 或没接对象存储时不该起协程，done 要立刻关闭
	//（否则 main 关停时会干等一个永远不会跑起来的 worker）。
	done := StartVideoProcessor(context.Background(), &svc.ServiceContext{})
	select {
	case _, ok := <-done:
		if ok {
			t.Fatal("done 是 struct{} 通道，不该收到值")
		}
	case <-time.After(time.Second):
		t.Fatal("依赖缺失时 worker 没有立即退出")
	}
}
