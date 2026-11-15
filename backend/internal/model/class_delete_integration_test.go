package model

// 删班级联清理的回归测试。
//
// 这块以前是漏的：DeleteClass 只删 members/courses/announcements/classes 四张表，
// 作业提交、考试、讨论、签到、成绩册全成了孤儿数据 —— 页面上看不到，
// 表却一直在长。这个用例把"每个从属表都造一条、删完逐个查 0"固定下来，
// 以后谁加一张带 class_id 的新表忘了同步，这里就会红。
//
// 纯 MySQL 用例，连不上库自动 skip（与其他 model 集成测试同一套约定）。

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDeleteClassCascadesAllChildren(t *testing.T) {
	ctx := context.Background()
	conn := newModelTestConn(t)
	cm := NewClassModel(conn)

	suffix := time.Now().UnixNano() % 1000000
	var uid int64
	if err := conn.QueryRowCtx(ctx, &uid, "SELECT id FROM users ORDER BY id LIMIT 1"); err != nil {
		t.Skipf("跳过：库里没有用户可挂数据（%v）", err)
	}

	className := fmt.Sprintf("集成测试班_%d_%d", suffix, time.Now().UnixNano()%1000)
	classId := mustExec(t, conn,
		"INSERT INTO classes (name, grade, major, head_teacher_id, created_by) VALUES (?,?,?,?,?)",
		className, "2026级", "测试", uid, "integration-test")
	otherClassId := mustExec(t, conn,
		"INSERT INTO classes (name, grade, major, head_teacher_id, created_by) VALUES (?,?,?,?,?)",
		className+"_对照", "2026级", "测试", uid, "integration-test")

	// 兜底：用例中途失败时也要把两张测试班清干净（DeleteClass 幂等，重复调不报错）
	t.Cleanup(func() {
		_, _ = cm.DeleteClass(classId)
		_, _ = cm.DeleteClass(otherClassId)
	})

	// ---- 造数据：每张从属表一条 ----
	mustExec(t, conn, "INSERT INTO class_members (class_id, user_id, role) VALUES (?,?,?)", classId, uid, "teacher")
	mustExec(t, conn, "INSERT INTO class_courses (class_id, course_id) VALUES (?,?)", classId, 1)
	mustExec(t, conn,
		"INSERT INTO announcements (class_id, title, content, publisher_id) VALUES (?,?,?,?)",
		classId, "测试公告", "正文", uid)
	mustExec(t, conn, "INSERT INTO class_grade_rules (class_id) VALUES (?)", classId)
	mustExec(t, conn,
		"INSERT INTO class_invitations (class_id, code, max_uses, used_count) VALUES (?,?,?,?)",
		classId, fmt.Sprintf("%06d", suffix), 0, 0)

	assignmentId := mustExec(t, conn,
		"INSERT INTO assignments (class_id, title, description, publisher_id) VALUES (?,?,?,?)",
		classId, "测试作业", "描述", uid)
	mustExec(t, conn,
		"INSERT INTO assignment_questions (assignment_id, type, stem, options, answer, score) VALUES (?,?,?,?,?,?)",
		assignmentId, "single", "题干", "[]", "A", 5)
	submissionId := mustExec(t, conn,
		"INSERT INTO assignment_submissions (assignment_id, user_id, content) VALUES (?,?,?)",
		assignmentId, uid, "答案")
	mustExec(t, conn,
		"INSERT INTO assignment_ai_grades (submission_id, assignment_id, status, score) VALUES (?,?,?,?)",
		submissionId, assignmentId, "done", 5)

	discussionId := mustExec(t, conn,
		"INSERT INTO discussions (class_id, user_id, title, content) VALUES (?,?,?,?)",
		classId, uid, "测试讨论", "正文")
	mustExec(t, conn, "INSERT INTO discussion_replies (discussion_id, user_id, content) VALUES (?,?,?)",
		discussionId, uid, "回复")

	signinId := mustExec(t, conn,
		"INSERT INTO class_signin (class_id, teacher_id, code, expires_at) VALUES (?,?,?,?)",
		classId, uid, fmt.Sprintf("%06d", suffix), time.Now().Add(time.Hour))
	mustExec(t, conn, "INSERT INTO signin_records (signin_id, user_id) VALUES (?,?)", signinId, uid)

	examId := mustExec(t, conn,
		"INSERT INTO exams (class_id, title, description, start_at, end_at, created_by) VALUES (?,?,?,?,?,?)",
		classId, "测试考试", "描述", time.Now(), time.Now().Add(time.Hour), uid)
	mustExec(t, conn,
		"INSERT INTO exam_questions (exam_id, type, stem, options, answer, score) VALUES (?,?,?,?,?,?)",
		examId, "essay", "主观题", "[]", "", 10)
	mustExec(t, conn,
		"INSERT INTO exam_submissions (exam_id, user_id, answers, status) VALUES (?,?,?,?)",
		examId, uid, "{}", "submitted")

	itemId := mustExec(t, conn,
		"INSERT INTO grade_items (class_id, source_type, source_id, title, max_score) VALUES (?,?,?,?,?)",
		classId, "assignment", assignmentId, "测试作业", 100)
	mustExec(t, conn, "INSERT INTO grade_scores (item_id, user_id, score) VALUES (?,?,?)", itemId, uid, 88)

	// 两条通知：一条指本班（该删），一条指向 "本班 id + 0" 这个别的班（不能误删）
	notifId := mustExec(t, conn,
		"INSERT INTO notifications (user_id, type, title, content, link_url) VALUES (?,?,?,?,?)",
		uid, "announcement", "本班公告", "", fmt.Sprintf("/class/%d", classId))
	keepNotifId := mustExec(t, conn,
		"INSERT INTO notifications (user_id, type, title, content, link_url) VALUES (?,?,?,?,?)",
		uid, "announcement", "别班公告", "", fmt.Sprintf("/class/%d0", classId))

	// ---- 删除 ----
	stats, err := cm.DeleteClass(classId)
	if err != nil {
		t.Fatalf("删除班级失败: %v", err)
	}
	if stats.Members != 1 || stats.Assignments != 1 || stats.Submissions != 1 ||
		stats.Discussions != 1 || stats.Signins != 1 || stats.Exams != 1 || stats.ExamSubmissions != 1 {
		t.Fatalf("删除统计不符合预期: %+v", stats)
	}

	// ---- 逐表断言已清空 ----
	checks := []struct {
		name  string
		query string
		arg   interface{}
	}{
		{"班级", "SELECT COUNT(*) FROM classes WHERE id=?", classId},
		{"成员", "SELECT COUNT(*) FROM class_members WHERE class_id=?", classId},
		{"绑定课程", "SELECT COUNT(*) FROM class_courses WHERE class_id=?", classId},
		{"公告", "SELECT COUNT(*) FROM announcements WHERE class_id=?", classId},
		{"邀请码", "SELECT COUNT(*) FROM class_invitations WHERE class_id=?", classId},
		{"作业", "SELECT COUNT(*) FROM assignments WHERE class_id=?", classId},
		{"作业题目", "SELECT COUNT(*) FROM assignment_questions WHERE assignment_id=?", assignmentId},
		{"作业提交", "SELECT COUNT(*) FROM assignment_submissions WHERE assignment_id=?", assignmentId},
		{"AI 初评", "SELECT COUNT(*) FROM assignment_ai_grades WHERE assignment_id=?", assignmentId},
		{"讨论", "SELECT COUNT(*) FROM discussions WHERE class_id=?", classId},
		{"讨论回复", "SELECT COUNT(*) FROM discussion_replies WHERE discussion_id=?", discussionId},
		{"签到", "SELECT COUNT(*) FROM class_signin WHERE class_id=?", classId},
		{"签到记录", "SELECT COUNT(*) FROM signin_records WHERE signin_id=?", signinId},
		{"考试", "SELECT COUNT(*) FROM exams WHERE class_id=?", classId},
		{"考试题目", "SELECT COUNT(*) FROM exam_questions WHERE exam_id=?", examId},
		{"考试提交", "SELECT COUNT(*) FROM exam_submissions WHERE exam_id=?", examId},
		{"成绩权重", "SELECT COUNT(*) FROM class_grade_rules WHERE class_id=?", classId},
		{"成绩项", "SELECT COUNT(*) FROM grade_items WHERE class_id=?", classId},
		{"成绩分数", "SELECT COUNT(*) FROM grade_scores WHERE item_id=?", itemId},
		{"班级通知", "SELECT COUNT(*) FROM notifications WHERE id=?", notifId},
	}
	for _, c := range checks {
		var n int64
		if err := conn.QueryRowCtx(ctx, &n, c.query, c.arg); err != nil {
			t.Fatalf("查询%s数量失败: %v", c.name, err)
		}
		if n != 0 {
			t.Fatalf("%s 未清理，还剩 %d 条", c.name, n)
		}
	}

	// ---- 不该被误伤的数据 ----
	var keep int64
	if err := conn.QueryRowCtx(ctx, &keep, "SELECT COUNT(*) FROM notifications WHERE id=?", keepNotifId); err != nil {
		t.Fatalf("查询对照通知失败: %v", err)
	}
	if keep != 1 {
		t.Fatalf("指向 /class/%d0 的通知被误删了（前缀匹配过于宽松）", classId)
	}
	if _, err := cm.GetClass(otherClassId); err != nil {
		t.Fatalf("对照班级被误删: %v", err)
	}
	_, _ = conn.ExecCtx(ctx, "DELETE FROM notifications WHERE id=?", keepNotifId)
}
