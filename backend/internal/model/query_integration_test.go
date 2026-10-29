package model

// 查询优化的集成回归测试：验证"批量聚合"改写后 SQL 的语义与字段映射。
//
// 这些用例需要真实 MySQL（表结构由 Flyway 迁移建好）：
//   - TEST_MYSQL_DSN 有值就连它，否则连本机默认的 3316（与 CI service container 对齐）；
//   - 连不上整条用例 skip，保证无依赖环境下 `go test ./...` 仍是绿的。
//
// 覆盖点：
//  1. AnnouncementReadStats 与逐条 ListReadUserIds 的结果一致；
//  2. ListGradebookRaw 的预聚合数字等于逐项建账的期望值（含"没有成绩的类别为 0"）；
//  3. ListQuestionsByExamIds 按考试分组、按 order_num 排序；
//  4. ListStartedSubmissions 把草稿 answers 一起带出（收卷时不再逐份回查）。

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func newModelTestConn(t *testing.T) sqlx.SqlConn {
	t.Helper()
	ds := os.Getenv("TEST_MYSQL_DSN")
	if ds == "" {
		ds = "root:Java666@tcp(127.0.0.1:3316)/learningplatform?charset=utf8mb4&parseTime=true&loc=Local"
	}
	conn := sqlx.NewMysql(ds)
	var one int
	err := conn.QueryRowCtx(context.Background(), &one, "SELECT 1")
	if err != nil {
		t.Skipf("跳过：连不上测试数据库（%v）。可用 TEST_MYSQL_DSN 覆盖。", err)
	}
	return conn
}

func mustExec(t *testing.T, conn sqlx.SqlConn, query string, args ...interface{}) int64 {
	t.Helper()
	res, err := conn.ExecCtx(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("执行失败: %v\nSQL: %s", err, query)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestQueryOptimizations_Integration(t *testing.T) {
	conn := newModelTestConn(t)
	cm := NewClassModel(conn)
	suffix := time.Now().UnixNano()
	userA := "perfa_" + time.Now().Format("150405") + "_" + intToStr(suffix%100000)
	userB := "perfb_" + time.Now().Format("150405") + "_" + intToStr(suffix%100000)

	uidA := mustExec(t, conn,
		"INSERT INTO users (username, password, nickname, role) VALUES (?,?,?,?)",
		userA, "x", userA, "student")
	uidB := mustExec(t, conn,
		"INSERT INTO users (username, password, nickname, role) VALUES (?,?,?,?)",
		userB, "x", userB, "student")
	classId := mustExec(t, conn,
		"INSERT INTO classes (name, grade, major, head_teacher_id, created_by) VALUES (?,?,?,?,?)",
		"perf-"+intToStr(suffix), "", "", uidA, userA)

	// 清理顺序与外键无关（这些表都没有外键），按"子表 → 主表"删。
	t.Cleanup(func() {
		c := context.Background()
		for _, q := range []struct {
			sql  string
			args []interface{}
		}{
			{"DELETE FROM grade_scores WHERE user_id IN (?,?)", []interface{}{uidA, uidB}},
			{"DELETE FROM grade_items WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM video_progress WHERE user_id IN (?,?)", []interface{}{uidA, uidB}},
			{"DELETE FROM signin_records WHERE user_id IN (?,?)", []interface{}{uidA, uidB}},
			{"DELETE FROM class_signin WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM announcement_reads WHERE user_id IN (?,?)", []interface{}{uidA, uidB}},
			{"DELETE FROM announcements WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM class_courses WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM course_videos WHERE name LIKE ?", []interface{}{"perf-video-" + intToStr(suffix) + "%"}},
			{"DELETE FROM courses WHERE name LIKE ?", []interface{}{"perf-course-" + intToStr(suffix) + "%"}},
			{"DELETE FROM assignments WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM exam_submissions WHERE exam_id IN (SELECT id FROM exams WHERE class_id=?)", []interface{}{classId}},
			{"DELETE FROM exam_questions WHERE exam_id IN (SELECT id FROM exams WHERE class_id=?)", []interface{}{classId}},
			{"DELETE FROM exams WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM class_members WHERE class_id=?", []interface{}{classId}},
			{"DELETE FROM classes WHERE id=?", []interface{}{classId}},
			{"DELETE FROM users WHERE id IN (?,?)", []interface{}{uidA, uidB}},
		} {
			_, _ = conn.ExecCtx(c, q.sql, q.args...)
		}
	})

	mustExec(t, conn, "INSERT INTO class_members (class_id, user_id, role) VALUES (?,?,?)", classId, uidA, "student")
	mustExec(t, conn, "INSERT INTO class_members (class_id, user_id, role) VALUES (?,?,?)", classId, uidB, "student")

	// ===== 1. 公告已读批量聚合 =====
	a1 := mustExec(t, conn, "INSERT INTO announcements (class_id, title, content, publisher_id) VALUES (?,?,?,?)", classId, "a1", "", uidA)
	a2 := mustExec(t, conn, "INSERT INTO announcements (class_id, title, content, publisher_id) VALUES (?,?,?,?)", classId, "a2", "", uidA)
	a3 := mustExec(t, conn, "INSERT INTO announcements (class_id, title, content, publisher_id) VALUES (?,?,?,?)", classId, "a3", "", uidA)
	mustExec(t, conn, "INSERT INTO announcement_reads (announcement_id, user_id) VALUES (?,?)", a1, uidA)
	mustExec(t, conn, "INSERT INTO announcement_reads (announcement_id, user_id) VALUES (?,?)", a1, uidB)
	mustExec(t, conn, "INSERT INTO announcement_reads (announcement_id, user_id) VALUES (?,?)", a2, uidB)

	stats, err := cm.AnnouncementReadStats(uidA, []int64{a1, a2, a3})
	if err != nil {
		t.Fatalf("AnnouncementReadStats: %v", err)
	}
	if got := stats[a1]; got.ReadCount != 2 || !got.ReadByMe {
		t.Fatalf("a1 期望 count=2 readByMe=true，实际 %+v", got)
	}
	if got := stats[a2]; got.ReadCount != 1 || got.ReadByMe {
		t.Fatalf("a2 期望 count=1 readByMe=false，实际 %+v", got)
	}
	if _, ok := stats[a3]; ok {
		t.Fatalf("a3 没有已读记录，不应出现在结果里: %+v", stats[a3])
	}
	// 与逐条查询交叉验证（防止聚合口径和明细接口漂移）
	for _, id := range []int64{a1, a2, a3} {
		ids, err := cm.ListReadUserIds(id)
		if err != nil {
			t.Fatalf("ListReadUserIds: %v", err)
		}
		if ids2 := stats[id]; ids2.ReadCount != len(ids) {
			t.Fatalf("公告 %d 聚合计数 %d ≠ 明细 %d", id, ids2.ReadCount, len(ids))
		}
	}

	// ===== 2. 成绩册预聚合 =====
	courseId := mustExec(t, conn, "INSERT INTO courses (name, intro, teacher, created_by) VALUES (?,?,?,?)",
		"perf-course-"+intToStr(suffix), "", userA, userA)
	mustExec(t, conn, "INSERT INTO class_courses (class_id, course_id) VALUES (?,?)", classId, courseId)
	videoId := mustExec(t, conn, "INSERT INTO course_videos (course_id, name, url, object_name) VALUES (?,?,?,?)",
		courseId, "perf-video-"+intToStr(suffix), "/v.mp4", "videos/x.mp4")
	mustExec(t, conn, `INSERT INTO video_progress (user_id, course_id, video_id, completed) VALUES (?,?,?,1)`,
		uidA, courseId, videoId)

	signinId := mustExec(t, conn, "INSERT INTO class_signin (class_id, title, start_at, teacher_id, code, expires_at) VALUES (?,?,NOW(),?,?,NOW())",
		classId, "s1", uidA, "perf"+intToStr(suffix)[:6])
	mustExec(t, conn, "INSERT INTO signin_records (signin_id, user_id) VALUES (?,?)", signinId, uidA)

	asgId := mustExec(t, conn, "INSERT INTO assignments (class_id, title, description, total_score, publisher_id) VALUES (?,?,?,?,?)",
		classId, "hw1", "", 100, uidA)
	asgItem := mustExec(t, conn, "INSERT INTO grade_items (class_id, source_type, source_id, title, max_score) VALUES (?,?,?,?,?)",
		classId, "assignment", asgId, "hw1", 100)
	mustExec(t, conn, "INSERT INTO grade_scores (item_id, user_id, score) VALUES (?,?,?)", asgItem, uidA, 80)

	examId := mustExec(t, conn, "INSERT INTO exams (class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by) VALUES (?,?,?,NOW(),NOW(),60,100,?)",
		classId, "exam1", "", uidA)
	examItem := mustExec(t, conn, "INSERT INTO grade_items (class_id, source_type, source_id, title, max_score) VALUES (?,?,?,?,?)",
		classId, "exam", examId, "exam1", 100)
	mustExec(t, conn, "INSERT INTO grade_scores (item_id, user_id, score) VALUES (?,?,?)", examItem, uidB, 60)

	rows, err := cm.ListGradebookRaw(classId)
	if err != nil {
		t.Fatalf("ListGradebookRaw: %v", err)
	}
	byUser := make(map[int64]*GradebookRaw, len(rows))
	for _, r := range rows {
		byUser[r.UserId] = r
	}
	ra, rb := byUser[uidA], byUser[uidB]
	if ra == nil || rb == nil {
		t.Fatalf("成绩册应包含两名学生，实际 %d 行", len(rows))
	}
	if ra.AssignScore != 80 || ra.AssignMax != 100 || ra.AssignCnt != 1 {
		t.Fatalf("A 的作业聚合不对: %+v", ra)
	}
	if ra.SigninCnt != 1 || ra.SigninTotal != 1 {
		t.Fatalf("A 的签到聚合不对: %+v", ra)
	}
	if ra.VideoDone != 1 || ra.VideoTotal != 1 {
		t.Fatalf("A 的视频聚合不对: %+v", ra)
	}
	if ra.ExamMax != 0 || ra.ExamScore != 0 {
		t.Fatalf("A 没有考试成绩，考试项应为 0: %+v", ra)
	}
	if rb.ExamScore != 60 || rb.ExamMax != 100 {
		t.Fatalf("B 的考试聚合不对: %+v", rb)
	}
	if rb.AssignMax != 0 || rb.SigninCnt != 0 || rb.VideoDone != 0 {
		t.Fatalf("B 没有作业/签到/视频数据，应为 0: %+v", rb)
	}

	// ===== 3. 题目批量分组 =====
	q1 := mustExec(t, conn, "INSERT INTO exam_questions (exam_id, type, stem, options, answer, score, order_num) VALUES (?,?,?,?,?,?,?)",
		examId, "single", "q2", "[]", "A", 5, 2)
	q2 := mustExec(t, conn, "INSERT INTO exam_questions (exam_id, type, stem, options, answer, score, order_num) VALUES (?,?,?,?,?,?,?)",
		examId, "single", "q1", "[]", "A", 5, 1)
	exam2 := mustExec(t, conn, "INSERT INTO exams (class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by) VALUES (?,?,?,NOW(),NOW(),60,100,?)",
		classId, "exam2", "", uidA)
	q3 := mustExec(t, conn, "INSERT INTO exam_questions (exam_id, type, stem, options, answer, score, order_num) VALUES (?,?,?,?,?,?,?)",
		exam2, "judge", "j1", "[]", "true", 5, 1)

	qs, err := NewExamModel(conn).ListQuestionsByExamIds([]int64{examId, exam2})
	if err != nil {
		t.Fatalf("ListQuestionsByExamIds: %v", err)
	}
	if len(qs[examId]) != 2 || qs[examId][0].Id != q2 || qs[examId][1].Id != q1 {
		t.Fatalf("exam1 题目应按 order_num 排序为 [%d,%d]，实际 %+v", q2, q1, qs[examId])
	}
	if len(qs[exam2]) != 1 || qs[exam2][0].Id != q3 {
		t.Fatalf("exam2 题目不对: %+v", qs[exam2])
	}

	// ===== 4. 收卷扫描带出草稿 =====
	mustExec(t, conn, `INSERT INTO exam_submissions (exam_id, user_id, answers, status, started_at) VALUES (?,?,?,'started',NOW())`,
		examId, uidA, `{"`+intToStr(q2)+`":"A"}`)
	overdue, err := NewExamModel(conn).ListStartedSubmissions()
	if err != nil {
		t.Fatalf("ListStartedSubmissions: %v", err)
	}
	found := false
	for _, r := range overdue {
		if r.ExamId == examId && r.UserId == uidA {
			found = true
			if r.Answers == "" {
				t.Fatal("扫描结果必须带出 answers 草稿，否则收卷会按 0 分处理")
			}
		}
	}
	if !found {
		t.Fatal("没有扫描到刚插入的 started 答卷")
	}
}

// intToStr 避免为拼测试数据引入 strconv 的噪声（只处理非负 int64）。
func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
