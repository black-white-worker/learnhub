package model

// 课程表查询的扫描回归测试。
//
// 起因（2026-11-07 线上事故）：CourseRow 有 8 个 db 字段（含 video_count），
// 但 FindCourseById / ListCourses 只 SELECT 7 列。go-zero 的 QueryRowCtx / QueryRowsCtx
// 是严格扫描（列数 < 字段数直接返回 ErrNotMatchDestination，跟数据无关），
// 于是"每一门课都查不到"：
//   - /api/course/detail 全线 404；
//   - 媒体鉴权拿不到课程 → 视频、讲义全部访问失败；
//   - 孤儿清理器的引用集合变成空集 → 把课程的封面和视频真删了。
//
// 这里锁住两件事：少列查询必须能扫进 CourseRow，以及"列数与字段数一致"这条
// 结构约束本身（用反射比对，新增字段却忘了改 SELECT 时测试先红）。

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestCourseRowColumnCount 静态检查：
// 不连库也能挡住"给 CourseRow 加字段但漏改某条 SELECT"这类回归。
func TestCourseRowColumnCount(t *testing.T) {
	n := 0
	rt := reflect.TypeOf(CourseRow{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		n++
	}
	// 只有 ListCoursesPaged 取 video_count；其余两条查询刻意不带它，
	// 所以必须用 QueryRowPartialCtx / QueryRowsPartialCtx（非严格扫描）。
	// 把"字段总数 - 1"钉死成 7：将来加字段时必须同时更新那两条 7 列 SELECT 和本断言。
	if got := n - 1; got != 7 {
		t.Fatalf("CourseRow 除 video_count 外应为 7 列，实际 %d —— 请同步 course.go 的两条 7 列 SELECT 与本断言", got)
	}
}

// TestVideoColumnsMatchVideoRow V24 给 course_videos 加了 3 个后处理状态列。
// ListVideos / FindVideo / ListPendingVideos 共用 videoColumns，且是严格扫描
// （QueryRowsCtx / QueryRowCtx）—— 列名或列数和 VideoRow 的 db tag 对不上，
// 三条查询会一起挂掉，表现和 2026-11-07 那次事故一样。
// 不连库也能钉死这条结构约束。
func TestVideoColumnsMatchVideoRow(t *testing.T) {
	fields := map[string]bool{}
	rt := reflect.TypeOf(VideoRow{})
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("db")
		if tag == "" || tag == "-" {
			continue
		}
		fields[tag] = false
	}

	cols := strings.Split(videoColumns, ",")
	if len(cols) != len(fields) {
		t.Fatalf("videoColumns 有 %d 列，VideoRow 有 %d 个 db 字段，严格扫描会直接报错", len(cols), len(fields))
	}
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if _, ok := fields[c]; !ok {
			t.Fatalf("videoColumns 里的列 %q 在 VideoRow 上没有对应 db tag", c)
		}
		fields[c] = true
	}
	for name, seen := range fields {
		if !seen {
			t.Fatalf("VideoRow 字段 %q 没被 videoColumns 取到", name)
		}
	}
}

// TestCourseQueriesScanWithoutVideoCount 真连库验证：7 列查询能扫进 8 字段的 CourseRow。
func TestCourseQueriesScanWithoutVideoCount(t *testing.T) {
	conn := newModelTestConn(t)
	m := NewCourseModel(conn)
	name := "scanreg-course-" + intToStr(time.Now().UnixNano()%10000000)

	id, err := m.InsertCourse(name, "回归用简介", "回归老师", "scanreg")
	if err != nil {
		t.Fatalf("造课程失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.ExecCtx(context.Background(), "DELETE FROM courses WHERE id=?", id)
	})

	row, err := m.FindCourseById(id)
	if err != nil {
		t.Fatalf("FindCourseById 不该因为少一列就报错（严格扫描会返回 ErrNotMatchDestination）: %v", err)
	}
	if row.Name != name {
		t.Fatalf("FindCourseById 取错了行，期望 %q 实际 %q", name, row.Name)
	}
	if row.VideoCount != 0 {
		t.Fatalf("本查询不取 video_count，应保持零值，实际 %d", row.VideoCount)
	}

	rows, err := m.ListCourses()
	if err != nil {
		t.Fatalf("ListCourses 报错: %v", err)
	}
	var found *CourseRow
	for _, r := range rows {
		if r.Id == id {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatalf("ListCourses 结果里没有刚建的课程 %d（共 %d 行）", id, len(rows))
	}
	if !strings.Contains(found.Intro, "回归") {
		t.Fatalf("ListCourses 字段映射不对: %+v", found)
	}

	// 8 列版本（带 video_count 子查询）同样要能扫，它和上面两条共用 CourseRow。
	// 它按 id 倒序，新课一定落在第一页。
	paged, total, err := m.ListCoursesPaged(1, 200)
	if err != nil {
		t.Fatalf("ListCoursesPaged 报错: %v", err)
	}
	if total < 1 {
		t.Fatalf("ListCoursesPaged 总数不对: %d", total)
	}
	var inPaged *CourseRow
	for _, r := range paged {
		if r.Id == id {
			inPaged = r
			break
		}
	}
	if inPaged == nil {
		t.Fatalf("ListCoursesPaged 第一页里没有刚建的课程 %d（共 %d 行）", id, len(paged))
	}
	if inPaged.VideoCount != 0 {
		t.Fatalf("新课视频数应为 0，实际 %d", inPaged.VideoCount)
	}
}
