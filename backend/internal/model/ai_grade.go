// 主观题 AI 初评结果的持久化（对应 Flyway V25 的 assignment_ai_grades 表）。
//
// 这份数据的定位要说清楚：它是**建议**，不是成绩。
// assignment_submissions.score / 成绩册里的分数只能由老师落笔（走 /grade 接口），
// AI 结果只存在这张表里，老师看完点"采纳"才写进成绩册。
package model

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// AI 初评状态（与 ai-service 的任务状态同名，便于对账）。
const (
	AIGradeProcessing = "processing"
	AIGradeDone       = "done"
	AIGradeFailed     = "failed"
	AIGradeManual     = "manual" // AI 解析不出来，等人工批改
)

// AIGradeRow 一份提交的最新 AI 初评。
type AIGradeRow struct {
	SubmissionId int64  `db:"submission_id" json:"submissionId"`
	AssignmentId int64  `db:"assignment_id" json:"assignmentId"`
	CourseId     int64  `db:"course_id" json:"courseId"`
	StudentId    int64  `db:"student_id" json:"studentId"`
	TaskId       string `db:"task_id" json:"taskId"`
	Status       string `db:"status" json:"status"`
	Score        int    `db:"score" json:"score"`
	MaxScore     int    `db:"max_score" json:"maxScore"`
	Comment      string `db:"comment" json:"comment"`
	// Items 逐题结果的 JSON 串，由 logic 层解成结构化对象再返回给前端
	Items string `db:"items" json:"-"`
	// ErrorText：数据库列名是保留字-ish 的 error，Go 字段不能叫 Error（和 error 接口混），
	// JSON 里仍输出 error，前端拿得到。
	ErrorText   string       `db:"error" json:"error,omitempty"`
	RequestedBy int64        `db:"requested_by" json:"requestedBy"`
	CreatedAt   sql.NullTime `db:"created_at" json:"-"`
	UpdatedAt   sql.NullTime `db:"updated_at" json:"updatedAt"`
}

const aiGradeColumns = `submission_id, assignment_id, course_id, student_id, task_id, status,
	score, max_score, comment, IFNULL(items,'') AS items, IFNULL(error,'') AS error, requested_by, created_at, updated_at`

type AIGradeModel struct {
	conn sqlx.SqlConn
}

func NewAIGradeModel(conn sqlx.SqlConn) *AIGradeModel {
	return &AIGradeModel{conn: conn}
}

// Upsert 写入/覆盖一份提交的最新 AI 初评（主键是 submission_id，一份提交只留最新一条）。
// created_at 不在更新列里，保留第一次发起的时间。
func (m *AIGradeModel) Upsert(r *AIGradeRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO assignment_ai_grades
		 (submission_id, assignment_id, course_id, student_id, task_id, status, score, max_score, comment, items, error, requested_by)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
			assignment_id=VALUES(assignment_id), course_id=VALUES(course_id), student_id=VALUES(student_id),
			task_id=VALUES(task_id), status=VALUES(status), score=VALUES(score),
			max_score=VALUES(max_score), comment=VALUES(comment), items=VALUES(items),
			error=VALUES(error), requested_by=VALUES(requested_by), updated_at=NOW()`,
		r.SubmissionId, r.AssignmentId, r.CourseId, r.StudentId, r.TaskId, r.Status,
		r.Score, r.MaxScore, r.Comment, r.Items, r.ErrorText, r.RequestedBy)
	return err
}

// UpdateStatus 只改状态与错误原因：入队失败 / worker 死信时把卡在 processing 的记录标掉。
// 分数、评语、逐题结果一律不碰 —— 状态回退不能把已有结果抹成 0。
func (m *AIGradeModel) UpdateStatus(submissionId int64, status, errText string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE assignment_ai_grades SET status=?, error=?, updated_at=NOW() WHERE submission_id=?",
		status, errText, submissionId)
	return err
}

// GetBySubmission 取一份提交的最新 AI 初评；没有记录返回 ErrNotFound。
func (m *AIGradeModel) GetBySubmission(submissionId int64) (*AIGradeRow, error) {
	var row AIGradeRow
	err := m.conn.QueryRowCtx(context.Background(), &row,
		"SELECT "+aiGradeColumns+" FROM assignment_ai_grades WHERE submission_id=?", submissionId)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// GetByTask 按 ai-service 任务 id 反查：回调里只带 taskId 时用它定位提交。
func (m *AIGradeModel) GetByTask(taskId string) (*AIGradeRow, error) {
	if taskId == "" {
		return nil, ErrNotFound
	}
	var row AIGradeRow
	err := m.conn.QueryRowCtx(context.Background(), &row,
		"SELECT "+aiGradeColumns+" FROM assignment_ai_grades WHERE task_id=? LIMIT 1", taskId)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &row, nil
}

// ListBySubmissions 批量取多份提交的 AI 初评（批阅队列一次拉一叠，避免 N+1）。
// 没有记录的提交 id 不会出现在返回的 map 里。
func (m *AIGradeModel) ListBySubmissions(ids []int64) (map[int64]*AIGradeRow, error) {
	out := make(map[int64]*AIGradeRow, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	var rows []*AIGradeRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT "+aiGradeColumns+" FROM assignment_ai_grades WHERE submission_id IN ("+placeholders+")", args...)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.SubmissionId] = r
	}
	return out, nil
}

// CountByAssignmentStatus 某份作业各状态的数量（老师端显示"AI 初评进度 x/y"）。
func (m *AIGradeModel) CountByAssignmentStatus(assignmentId int64) (processing, done int, err error) {
	type countRow struct {
		Status string `db:"status"`
		N      int    `db:"n"`
	}
	var rows []countRow
	err = m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT status, COUNT(*) AS n FROM assignment_ai_grades WHERE assignment_id=? GROUP BY status",
		assignmentId)
	if err != nil {
		return 0, 0, err
	}
	for _, c := range rows {
		switch c.Status {
		case AIGradeProcessing:
			processing = c.N
		case AIGradeDone:
			done = c.N
		}
	}
	return processing, done, nil
}
