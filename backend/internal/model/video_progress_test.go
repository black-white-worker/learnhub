package model

import "testing"

// 用例：首次上报（没有历史记录）——观看时长就是本次增量，未看完。
func TestNextVideoProgress_FirstReport(t *testing.T) {
	got := NextVideoProgress(nil, 30, 30, 600)
	if got.PositionSeconds != 30 || got.WatchedSeconds != 30 || got.DurationSeconds != 600 {
		t.Fatalf("首次上报结果不对: %+v", got)
	}
	if got.Completed {
		t.Fatal("只看了 30/600 秒不应算完成")
	}
}

// 用例：累计观看时长要叠加历史值，而不是覆盖。
func TestNextVideoProgress_Accumulates(t *testing.T) {
	prev := &VideoProgressRow{WatchedSeconds: 500, DurationSeconds: 600}
	got := NextVideoProgress(prev, 520, 20, 600)
	if got.WatchedSeconds != 520 {
		t.Fatalf("应累加到 520，实际 %d", got.WatchedSeconds)
	}
	if got.Completed {
		t.Fatal("520/600 未达到 90%，不应完成")
	}
}

// 用例：累计观看封顶到视频总长，不会出现 700/600。
func TestNextVideoProgress_CapsAtDuration(t *testing.T) {
	prev := &VideoProgressRow{WatchedSeconds: 560, DurationSeconds: 600}
	got := NextVideoProgress(prev, 590, 100, 600)
	if got.WatchedSeconds != 600 {
		t.Fatalf("应封顶到 600，实际 %d", got.WatchedSeconds)
	}
	if !got.Completed {
		t.Fatal("达到总长应标记完成")
	}
}

// 用例：总时长取历史与本次的较大值（播放器上报 0 时不能把总长抹掉）。
func TestNextVideoProgress_KeepsKnownDuration(t *testing.T) {
	prev := &VideoProgressRow{WatchedSeconds: 100, DurationSeconds: 600}
	got := NextVideoProgress(prev, 100, 10, 0)
	if got.DurationSeconds != 600 {
		t.Fatalf("总时长应保留 600，实际 %d", got.DurationSeconds)
	}
}

// 用例：完成标记只增不减（乱序上报不能把"已看完"改回去）。
func TestNextVideoProgress_CompletedIsSticky(t *testing.T) {
	prev := &VideoProgressRow{WatchedSeconds: 600, DurationSeconds: 600, Completed: true}
	got := NextVideoProgress(prev, 10, 0, 0)
	if !got.Completed {
		t.Fatal("已完成记录不能因后续上报回退")
	}
}

// 用例：负数输入按 0 处理（脏数据不写库）。
func TestNextVideoProgress_ClampsNegative(t *testing.T) {
	got := NextVideoProgress(nil, -5, -10, -600)
	if got.PositionSeconds != 0 || got.WatchedSeconds != 0 || got.DurationSeconds != 0 {
		t.Fatalf("负数应归零: %+v", got)
	}
	if got.Completed {
		t.Fatal("0/0 不应算完成")
	}
}

// 用例：90% 阈值边界（54/60 完成，53/60 未完成）。
func TestNextVideoProgress_Threshold(t *testing.T) {
	done := NextVideoProgress(&VideoProgressRow{DurationSeconds: 60}, 54, 54, 60)
	if !done.Completed {
		t.Fatal("54/60 = 90% 应算完成")
	}
	notDone := NextVideoProgress(&VideoProgressRow{DurationSeconds: 60}, 53, 53, 60)
	if notDone.Completed {
		t.Fatal("53/60 不应算完成")
	}
}
