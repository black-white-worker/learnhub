package model

// AI 初评表的读写回归：V25 是新表，列名、占位符、UPSERT 的更新列都得真跑一遍才放心。
// 建表直接读迁移文件里的 V25 原文，保证测试用的结构和上线的一字不差。
//
// 没有可用 MySQL 时整条用例 skip（与 query_integration_test.go 同一套约定）。

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readV25DDL(t *testing.T) string {
	t.Helper()
	// 测试工作目录是 internal/model，迁移文件在 backend/deploy/sql/migration
	for _, p := range []string{
		filepath.Join("..", "deploy", "sql", "migration", "V25__assignment_ai_grades.sql"),
		filepath.Join("..", "..", "deploy", "sql", "migration", "V25__assignment_ai_grades.sql"),
	} {
		if b, err := os.ReadFile(p); err == nil {
			return string(b)
		}
	}
	t.Fatal("找不到 V25 迁移文件，无法建测试表")
	return ""
}

func TestAIGradeModel_Integration(t *testing.T) {
	conn := newModelTestConn(t)
	m := NewAIGradeModel(conn)

	if _, err := conn.ExecCtx(context.Background(), readV25DDL(t)); err != nil {
		t.Fatalf("建测试表失败: %v", err)
	}

	// 用 9000xxxxx 这段固定 id，避开真实数据；跑完清掉
	base := int64(900000001)
	ids := []int64{base, base + 1, base + 2, base + 3}
	clearRows := func() {
		for _, id := range ids {
			_, _ = conn.ExecCtx(context.Background(), "DELETE FROM assignment_ai_grades WHERE submission_id=?", id)
		}
	}
	clearRows()
	t.Cleanup(clearRows)

	row := &AIGradeRow{
		SubmissionId: base,
		AssignmentId: base + 10,
		CourseId:     7,
		StudentId:    8,
		TaskId:       "task-aaa",
		Status:       AIGradeProcessing,
		MaxScore:     30,
		RequestedBy:  99,
	}
	if err := m.Upsert(row); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}

	// 用例 1：processing 阶段只有满分，没有分数
	got, err := m.GetBySubmission(base)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Status != AIGradeProcessing || got.Score != 0 || got.MaxScore != 30 {
		t.Fatalf("初始状态不对: %+v", got)
	}
	if got.TaskId != "task-aaa" || got.StudentId != 8 || got.RequestedBy != 99 {
		t.Fatalf("冗余字段没存住: %+v", got)
	}
	if got.Items != "" {
		t.Fatalf("没写过逐题结果时应为空串，实际 %q", got.Items)
	}

	// 用例 2：回调结果覆盖同一行，不产生第二条（主键是 submission_id）
	row.Status = AIGradeDone
	row.Score = 26
	row.Comment = "AI 初评：2 道主观题"
	row.Items = `[{"questionId":1,"score":13,"maxScore":20,"parseOk":true}]`
	if err := m.Upsert(row); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}
	var count int
	if err := conn.QueryRowCtx(context.Background(), &count,
		"SELECT COUNT(*) FROM assignment_ai_grades WHERE submission_id=?", base); err != nil {
		t.Fatalf("计数失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("UPSERT 不该多出第二行，实际 %d 行", count)
	}
	got, err = m.GetBySubmission(base)
	if err != nil {
		t.Fatalf("回读失败: %v", err)
	}
	if got.Status != AIGradeDone || got.Score != 26 || !strings.Contains(got.Items, "questionId") {
		t.Fatalf("结果没覆盖: %+v", got)
	}

	// 用例 3：按 taskId 反查（回调只带 taskId 时的定位路径）
	byTask, err := m.GetByTask("task-aaa")
	if err != nil {
		t.Fatalf("按 taskId 反查失败: %v", err)
	}
	if byTask.SubmissionId != base {
		t.Fatalf("反查到了别的提交: %+v", byTask)
	}
	if _, err := m.GetByTask(""); err != ErrNotFound {
		t.Fatalf("空 taskId 应直接 ErrNotFound，实际 %v", err)
	}

	// 用例 4：UpdateStatus 只动状态与原因，不碰已拿到的分数
	if err := m.UpdateStatus(base, AIGradeManual, "AI 未能给出全部题目的分数"); err != nil {
		t.Fatalf("改状态失败: %v", err)
	}
	got, _ = m.GetBySubmission(base)
	if got.Status != AIGradeManual || got.ErrorText == "" {
		t.Fatalf("状态/原因没写进去: %+v", got)
	}
	if got.Score != 26 {
		t.Fatalf("UpdateStatus 不该把分数清零，实际 %d", got.Score)
	}

	// 用例 5：批量读（批阅队列）只返回存在的提交
	if err := m.Upsert(&AIGradeRow{SubmissionId: base + 1, AssignmentId: base + 10, TaskId: "task-bbb", Status: AIGradeDone, Score: 5, MaxScore: 10}); err != nil {
		t.Fatalf("第二条写入失败: %v", err)
	}
	if err := m.Upsert(&AIGradeRow{SubmissionId: base + 2, AssignmentId: base + 10, TaskId: "task-ccc", Status: AIGradeProcessing}); err != nil {
		t.Fatalf("第三条写入失败: %v", err)
	}
	rows, err := m.ListBySubmissions(ids)
	if err != nil {
		t.Fatalf("批量读取失败: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("应只返回存在的 3 条，实际 %d", len(rows))
	}
	if rows[base].Status != AIGradeManual || rows[base+1].Score != 5 {
		t.Fatalf("批量读的结果不对: %+v", rows)
	}
	if _, ok := rows[base+3]; ok {
		t.Fatal("不存在的提交不该出现在结果里")
	}
	empty, err := m.ListBySubmissions(nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("空入参应返回空 map: %v %v", empty, err)
	}

	// 用例 6：按作业统计进度（base=manual、base+1=done、base+2=processing）
	processing, done, err := m.CountByAssignmentStatus(base + 10)
	if err != nil {
		t.Fatalf("统计失败: %v", err)
	}
	if processing != 1 || done != 1 {
		t.Fatalf("期望 processing=1 done=1，实际 %d/%d", processing, done)
	}

	// 用例 7：不存在的提交返回 ErrNotFound（调用方据此显示"未发起"）
	if _, err := m.GetBySubmission(base + 3); err != ErrNotFound {
		t.Fatalf("期望 ErrNotFound，实际 %v", err)
	}

	// 用例 8：重跑同一份提交 —— 行被覆盖回 processing，taskId 反查仍指向这条
	if err := m.Upsert(&AIGradeRow{SubmissionId: base + 1, AssignmentId: base + 10, TaskId: "task-bbb", Status: AIGradeProcessing}); err != nil {
		t.Fatalf("重跑写入失败: %v", err)
	}
	again, err := m.GetByTask("task-bbb")
	if err != nil {
		t.Fatalf("重跑后反查失败: %v", err)
	}
	if again.Status != AIGradeProcessing {
		t.Fatalf("重跑应把状态回到 processing: %+v", again)
	}
}
