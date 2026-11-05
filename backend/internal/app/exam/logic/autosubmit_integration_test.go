package logic

// 超时自动收卷的集成测试：需要 MySQL（和 Redis 无关）。
//
// 覆盖的不变量：
//  1. 到期的 started 答卷会被收成 expired；
//  2. 收卷分数来自"最后一次草稿"，而不是 0（旧实现的白考问题）；
//  3. 收卷结果会写进成绩册（grade_items / grade_scores）；
//  4. 未到期的答卷不受影响。
//
// 依赖不可用时整条用例 skip，保持 `go test ./...` 在无库环境下是绿的；
// CI 里起 service containers 后会自动真跑。

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// newExamTestSvcCtx 连接测试库；连不上返回 nil。
//
// 自动收卷只依赖 MySQL（不碰 Redis），所以这里直接装配用得到的 Store，
// 不走 svc.NewServiceContext——那会用 MustNewRedis，在没有 Redis 的本机把测试打挂。
func newExamTestSvcCtx(t *testing.T) *svc.ServiceContext {
	t.Helper()
	ds := os.Getenv("TEST_MYSQL_DSN")
	if ds == "" {
		ds = "root:Java666@tcp(127.0.0.1:3316)/learningplatform?charset=utf8mb4&parseTime=true&loc=Local"
	}
	conn := sqlx.NewMysql(ds)
	var one int
	if err := conn.QueryRowCtx(context.Background(), &one, "SELECT 1"); err != nil {
		t.Skipf("跳过：连不上测试数据库（%v）。可用 TEST_MYSQL_DSN 覆盖。", err)
	}
	return &svc.ServiceContext{
		DB:          conn,
		UserStore:   model.NewUserModel(conn),
		ClassStore:  model.NewClassModel(conn),
		ExamStore:   model.NewExamModel(conn),
		CourseStore: model.NewCourseModel(conn),
	}
}

func TestRunAutoSubmitOnce_GradesSavedDraft(t *testing.T) {
	ctx := newExamTestSvcCtx(t)

	// ===== 造数据：用户 + 班级 + 已结束的考试 + 两道题 =====
	suffix := time.Now().UnixNano()
	username := fmt.Sprintf("autosubmit_%d", suffix)
	if err := ctx.UserStore.Add(username, "test123456"); err != nil {
		t.Fatalf("造用户失败: %v", err)
	}
	uid, err := ctx.UserStore.GetIdByUsername(username)
	if err != nil {
		t.Fatalf("取用户 id 失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM grade_scores WHERE user_id=?", uid)
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM grade_items WHERE title LIKE ?", "自动收卷测试%")
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM exam_submissions WHERE user_id=?", uid)
		_ = ctx.UserStore.Delete(username)
	})

	classId, err := ctx.ClassStore.InsertClass("自动收卷测试班", "", "", username, uid)
	if err != nil {
		t.Fatalf("造班级失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM classes WHERE id=?", classId)
	})

	// 考试：2 小时前开始、1 小时前结束、时长 30 分钟 → 一定超时
	now := time.Now()
	examId, err := ctx.ExamStore.InsertExam(&model.ExamRow{
		ClassId: classId, Title: "自动收卷测试卷", Description: "",
		StartAt: now.Add(-2 * time.Hour), EndAt: now.Add(-time.Hour),
		DurationMin: 30, TotalScore: 100, CreatedBy: uid,
	})
	if err != nil {
		t.Fatalf("造考试失败: %v", err)
	}
	t.Cleanup(func() {
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM exam_questions WHERE exam_id=?", examId)
		_, _ = ctx.DB.ExecCtx(context.Background(), "DELETE FROM exams WHERE id=?", examId)
	})

	q1, err := ctx.ExamStore.InsertQuestion(&model.ExamQuestionRow{
		ExamId: examId, Type: "single", Stem: "1+1=?", Options: `["2","3"]`, Answer: "2", Score: 40,
	})
	if err != nil {
		t.Fatalf("造题失败: %v", err)
	}
	if _, err := ctx.ExamStore.InsertQuestion(&model.ExamQuestionRow{
		ExamId: examId, Type: "judge", Stem: "地球是圆的", Answer: "true", Score: 60,
	}); err != nil {
		t.Fatalf("造题失败: %v", err)
	}

	// ===== 开考 + 存草稿（第一题对、第二题空）=====
	if err := ctx.ExamStore.StartSubmission(examId, uid, now.Add(-90*time.Minute)); err != nil {
		t.Fatalf("开考失败: %v", err)
	}
	draft := fmt.Sprintf(`{"%d":"2"}`, q1)
	ok, err := ctx.ExamStore.SaveDraft(examId, uid, draft)
	if err != nil || !ok {
		t.Fatalf("存草稿失败: ok=%v err=%v", ok, err)
	}

	// ===== 执行一轮收卷 =====
	RunAutoSubmitOnce(ctx)

	sub, err := ctx.ExamStore.GetMySubmission(examId, uid)
	if err != nil {
		t.Fatalf("读答卷失败: %v", err)
	}
	if sub.Status != model.SubmitStatusExpired {
		t.Fatalf("期望状态 expired，实际 %q", sub.Status)
	}
	if !sub.AutoScore.Valid || sub.AutoScore.Int64 != 40 {
		t.Fatalf("期望按草稿判 40 分，实际 %+v", sub.AutoScore)
	}

	// 成绩册必须同步落库（老师端看的是这张表）
	var gradeRow struct {
		Score float64 `db:"score"`
	}
	err = ctx.DB.QueryRowCtx(context.Background(), &gradeRow,
		`SELECT gs.score FROM grade_scores gs
		   JOIN grade_items gi ON gi.id = gs.item_id
		  WHERE gi.source_type='exam' AND gi.source_id=? AND gs.user_id=?`, examId, uid)
	if err != nil {
		t.Fatalf("成绩册没有写入记录: %v", err)
	}
	if gradeRow.Score != 40 {
		t.Fatalf("成绩册期望 40 分，实际 %v", gradeRow.Score)
	}

	// 已收卷的答卷不能再改草稿
	if ok, err := ctx.ExamStore.SaveDraft(examId, uid, `{"1":"x"}`); err != nil || ok {
		t.Fatalf("已收卷后 SaveDraft 应返回 false，实际 ok=%v err=%v", ok, err)
	}
}
