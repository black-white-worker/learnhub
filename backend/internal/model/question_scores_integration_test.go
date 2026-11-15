package model

// 逐题评分的落库回归：老师改完每道主观题的分数后，重新打开答卷要能原样读回来。
// 纯 MySQL 用例，连不上库自动 skip（与其他 model 集成测试同一套约定）。

import (
	"context"
	"testing"
	"time"
)

func TestGradeSubmissionPersistsQuestionScores(t *testing.T) {
	ctx := context.Background()
	conn := newModelTestConn(t)
	cm := NewClassModel(conn)

	var uid int64
	if err := conn.QueryRowCtx(ctx, &uid, "SELECT id FROM users ORDER BY id LIMIT 1"); err != nil {
		t.Skipf("跳过：库里没有用户可挂提交（%v）", err)
	}
	subId := mustExec(t, conn,
		"INSERT INTO assignment_submissions (assignment_id, user_id, content) VALUES (?,?,?)",
		900000000+time.Now().UnixNano()%100000, uid, "")
	defer func() { _, _ = conn.ExecCtx(ctx, "DELETE FROM assignment_submissions WHERE id=?", subId) }()

	if err := cm.GradeSubmission(subId, 17, "批语", `{"12":9,"13":8}`); err != nil {
		t.Fatalf("批改失败: %v", err)
	}
	row, err := cm.GetSubmission(subId)
	if err != nil {
		t.Fatalf("读回提交失败: %v", err)
	}
	if !row.Score.Valid || row.Score.Int64 != 17 {
		t.Fatalf("总分未保存: %+v", row.Score)
	}
	if row.QuestionScores != `{"12":9,"13":8}` {
		t.Fatalf("逐题分未保存: %q", row.QuestionScores)
	}
}

func TestExamGradeSubmissionPersistsQuestionScores(t *testing.T) {
	ctx := context.Background()
	conn := newModelTestConn(t)
	em := NewExamModel(conn)

	var uid int64
	if err := conn.QueryRowCtx(ctx, &uid, "SELECT id FROM users ORDER BY id LIMIT 1"); err != nil {
		t.Skipf("跳过：库里没有用户可挂提交（%v）", err)
	}
	// exam_id 用随机大数，避开 uk_exam_user 与真实考试的冲突（exam_id 无外键）
	examId := 900000000 + time.Now().UnixNano()%100000
	subId := mustExec(t, conn,
		"INSERT INTO exam_submissions (exam_id, user_id, answers, status) VALUES (?,?,'','submitted')",
		examId, uid)
	defer func() { _, _ = conn.ExecCtx(ctx, "DELETE FROM exam_submissions WHERE id=?", subId) }()

	if err := em.GradeSubmission(subId, 42, "批语", `{"21":8}`, uid); err != nil {
		t.Fatalf("批改失败: %v", err)
	}
	row, err := em.GetSubmission(subId)
	if err != nil {
		t.Fatalf("读回提交失败: %v", err)
	}
	if !row.TeacherScore.Valid || row.TeacherScore.Int64 != 42 {
		t.Fatalf("总分未保存: %+v", row.TeacherScore)
	}
	if row.QuestionScores != `{"21":8}` {
		t.Fatalf("逐题分未保存: %q", row.QuestionScores)
	}
}
