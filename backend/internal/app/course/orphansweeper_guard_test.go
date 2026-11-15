package course

// 孤儿清理器的三道硬闸的回归测试（2026-11-07 误删事故的直接防线）。
//
// 事故链：courses 查询列数不匹配报错 → Store.List() 把错误吞成空切片 →
// 引用集合为空 → 一门正常课程的封面/视频被当成孤儿删掉，桶没开版本，不可恢复。
// 结论：清理器唯一的安全来源是"引用集合可信"，不可信时一个文件都不能删。

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// listStubStore 只接管 List，其余方法委托真实 store。
// 引用集合从哪来是本组测试唯一的变量，不能靠内存 store 的预置课程碰运气。
type listStubStore struct {
	CourseStore
	courses []*Course
	err     error
}

func (s listStubStore) List() ([]*Course, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.courses, nil
}

func replaceStore(t *testing.T, st CourseStore) {
	t.Helper()
	old := Store
	Store = st
	t.Cleanup(func() { Store = old })
}

// putOldObject 造一个已超过安全窗口的托管对象，返回对象名。
func putOldObject(t *testing.T, f *fakeOss, name string) {
	t.Helper()
	if _, err := f.PutObject(context.Background(), name, strings.NewReader("x"), 1, "application/octet-stream"); err != nil {
		t.Fatalf("造对象 %s 失败: %v", name, err)
	}
	f.backdate(name, 30*time.Minute)
}

// 引用集合建不出来（查询报错）→ 整轮跳过，且必须把错误报出去。
func TestSweepOrphans_SkipsRoundWhenCourseListFails(t *testing.T) {
	ctx := newTestSvcCtx()
	f := ctx.Oss.(*fakeOss)
	orphan := "course/123/videos/should-survive.mp4"
	putOldObject(t, f, orphan)

	replaceStore(t, listStubStore{CourseStore: Store, err: errors.New("模拟课程查询失败")})

	if err := sweepOrphans(ctx); err == nil {
		t.Fatal("课程读不出来时必须返回错误，否则故障又会像上次一样被静默吞掉")
	}
	if len(f.removed) != 0 {
		t.Fatalf("引用集合不可信却删了 %d 个对象: %v", len(f.removed), f.removed)
	}
}

// 一门课都查不到但 MinIO 还有文件 → 更可能是连错库/空 schema，跳过。
func TestSweepOrphans_SkipsRoundWhenNoCourses(t *testing.T) {
	ctx := newTestSvcCtx()
	f := ctx.Oss.(*fakeOss)
	orphan := "course/124/videos/should-survive.mp4"
	putOldObject(t, f, orphan)

	replaceStore(t, listStubStore{CourseStore: Store, courses: nil})

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("空课程表不算错误，应返回 nil: %v", err)
	}
	if len(f.removed) != 0 {
		t.Fatalf("课程表为空却删了 %d 个对象: %v", len(f.removed), f.removed)
	}
}

// 熔断：一轮要删掉一半以上的托管对象（且不少于 10 个）→ 全轮作废。
// 真孤儿是零星的；成片说明引用集合和数据对不上。
func TestSweepOrphans_BreakerAbortsMassDelete(t *testing.T) {
	ctx := newTestSvcCtx()
	f := ctx.Oss.(*fakeOss)

	covered := "course/7/cover/original.png"
	putOldObject(t, f, covered)
	for i := 0; i < 12; i++ {
		putOldObject(t, f, "course/7/videos/v"+strconv.Itoa(i)+".mp4")
	}
	courses := []*Course{{
		Id:       7,
		CoverUrl: "http://fake-minio:9000/learningplatform/" + covered,
		Videos:   []*Video{}, Resources: []*Resource{},
	}}
	replaceStore(t, listStubStore{CourseStore: Store, courses: courses})

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("熔断不算错误，应返回 nil: %v", err)
	}
	if len(f.removed) != 0 {
		t.Fatalf("候选 12 个 / 托管 13 个已触发熔断，却删了 %d 个: %v", len(f.removed), f.removed)
	}
}

// 反向保护：零星孤儿 + 大量被引用对象时熔断不能误伤正常清理。
func TestSweepOrphans_BreakerAllowsNormalCleanup(t *testing.T) {
	ctx := newTestSvcCtx()
	f := ctx.Oss.(*fakeOss)

	var videos []*Video
	for i := 0; i < 15; i++ {
		name := "course/8/videos/keep" + strconv.Itoa(i) + ".mp4"
		putOldObject(t, f, name)
		videos = append(videos, &Video{ObjectName: name, Url: "http://fake-minio:9000/learningplatform/" + name})
	}
	// 15 个被引用 + 2 个真孤儿：占比 2/17，且不到最小批量 10 —— 应该照删
	orphanA := "course/8/videos/gone0.mp4"
	orphanB := "course/8/videos/gone1.mp4"
	putOldObject(t, f, orphanA)
	putOldObject(t, f, orphanB)

	replaceStore(t, listStubStore{CourseStore: Store, courses: []*Course{{
		Id: 8, Videos: videos, Resources: []*Resource{},
	}}})

	if err := sweepOrphans(ctx); err != nil {
		t.Fatalf("清理报错: %v", err)
	}
	if len(f.removed) != 2 {
		t.Fatalf("期望只删 2 个真孤儿，实际删了 %d 个: %v", len(f.removed), f.removed)
	}
}
