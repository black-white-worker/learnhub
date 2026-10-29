package model

// 「我的考试 / 我的作业」聚合查询。
//
// 背景：前端 ExamListView 原来是「先取我的班级 → 循环每个班级再取考试」，
// 5 个班就是 6 次往返，页面明显卡；而且每场考试的"我交了没"还要再查一次。
// 这里用一条 JOIN 把"我所在班级的考试/作业 + 我的提交状态"一次查出来。

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// MyExamRow 一场考试的"我的视角"：班级名 + 我的提交状态。
//
// 注意 MyStatus 只表示"我的参与情况"（有没有交卷），不表示考试本身开始没开始。
// 时间阶段由 Phase 表达（upcoming/ongoing/ended），两者必须分开：
// 改造前把"没有提交记录"直接当成"未开始"，导致已经结束的考试也显示"未开始"。
type MyExamRow struct {
	Id          int64         `db:"id" json:"id"`
	ClassId     int64         `db:"class_id" json:"classId"`
	ClassName   string        `db:"class_name" json:"className"`
	Title       string        `db:"title" json:"title"`
	StartAt     time.Time     `db:"start_at" json:"startAt"`
	EndAt       time.Time     `db:"end_at" json:"endAt"`
	DurationMin int           `db:"duration_minutes" json:"durationMinutes"`
	TotalScore  int           `db:"total_score" json:"totalScore"`
	MyRole      string        `db:"my_role" json:"myRole"`     // 我在这个班的角色（teacher/student/admin）
	MyStatus    string        `db:"my_status" json:"myStatus"` // "" 无提交记录 / started / submitted / expired
	MyScore     sql.NullInt64 `db:"my_score" json:"-"`
	SubmittedAt sql.NullTime  `db:"submitted_at" json:"-"`

	// Phase 由 handler 按当前时间计算：upcoming（未开始）/ ongoing（进行中）/ ended（已结束）
	Phase string `db:"-" json:"phase"`
	// CanAttend 学生且进行中且未交卷 → 可以去考试
	CanAttend bool `db:"-" json:"canAttend"`
}

// MarshalJSON 把 NullInt/NullTime 转成指针，前端拿到的就是数字或 null。
func (r *MyExamRow) MarshalJSON() ([]byte, error) {
	type Alias MyExamRow
	var score *int64
	if r.MyScore.Valid {
		v := r.MyScore.Int64
		score = &v
	}
	var submitted *string
	if r.SubmittedAt.Valid {
		v := r.SubmittedAt.Time.Format(time.RFC3339)
		submitted = &v
	}
	return json.Marshal(&struct {
		MyScore     *int64  `json:"myScore"`
		SubmittedAt *string `json:"submittedAt"`
		*Alias
	}{MyScore: score, SubmittedAt: submitted, Alias: (*Alias)(r)})
}

// MyAssignmentRow 一份作业的"我的视角"。
type MyAssignmentRow struct {
	Id           int64         `db:"id" json:"id"`
	ClassId      int64         `db:"class_id" json:"classId"`
	ClassName    string        `db:"class_name" json:"className"`
	Title        string        `db:"title" json:"title"`
	Description  string        `db:"description" json:"description"`
	Deadline     sql.NullTime  `db:"deadline" json:"-"`
	TotalScore   int           `db:"total_score" json:"totalScore"`
	SubmissionId sql.NullInt64 `db:"submission_id" json:"-"`
	MyScore      sql.NullInt64 `db:"my_score" json:"-"`
	SubmittedAt  sql.NullTime  `db:"submitted_at" json:"-"`
}

// MarshalJSON 同上；另外给前端一个可直接判断的 myStatus：
// submitted=已交，overdue=已过截止未交，pending=待完成。
func (r *MyAssignmentRow) MarshalJSON() ([]byte, error) {
	type Alias MyAssignmentRow
	now := time.Now()
	status := "pending"
	if r.SubmissionId.Valid {
		status = "submitted"
	} else if r.Deadline.Valid && r.Deadline.Time.Before(now) {
		status = "overdue"
	}
	var deadline, submitted *string
	if r.Deadline.Valid {
		v := r.Deadline.Time.Format(time.RFC3339)
		deadline = &v
	}
	if r.SubmittedAt.Valid {
		v := r.SubmittedAt.Time.Format(time.RFC3339)
		submitted = &v
	}
	var score *int64
	if r.MyScore.Valid {
		v := r.MyScore.Int64
		score = &v
	}
	return json.Marshal(&struct {
		Deadline    *string `json:"deadline"`
		SubmittedAt *string `json:"submittedAt"`
		MyScore     *int64  `json:"myScore"`
		MyStatus    string  `json:"myStatus"`
		*Alias
	}{Deadline: deadline, SubmittedAt: submitted, MyScore: score, MyStatus: status, Alias: (*Alias)(r)})
}

// ListMyExams 我所在班级的全部考试（含我的提交状态），按开考时间倒序。
//
// 说明：用 class_members 而不是 users.role 过滤——"我能看到哪些考试"
// 取决于我在哪些班级里，跟全局角色无关；管理员要总览用班级列表页。
func (m *ExamModel) ListMyExams(userId int64, limit int) ([]*MyExamRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []*MyExamRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows, `
		SELECT e.id, e.class_id, c.name AS class_name, e.title, e.start_at, e.end_at,
		       e.duration_minutes, e.total_score, cm.role AS my_role,
		       COALESCE(s.status, '') AS my_status,
		       COALESCE(s.teacher_score, s.auto_score) AS my_score,
		       s.submitted_at
		  FROM exams e
		  JOIN class_members cm ON cm.class_id = e.class_id AND cm.user_id = ?
		  JOIN classes c       ON c.id = e.class_id
		  LEFT JOIN exam_submissions s ON s.exam_id = e.id AND s.user_id = ?
		 ORDER BY e.start_at DESC
		 LIMIT ?`, userId, userId, limit)
	return rows, err
}

// ListMyAssignments 我所在班级的全部作业（含我的提交状态），按创建时间倒序。
func (m *ClassModel) ListMyAssignments(userId int64, limit int) ([]*MyAssignmentRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	var rows []*MyAssignmentRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows, `
		SELECT a.id, a.class_id, c.name AS class_name, a.title, a.description, a.deadline, a.total_score,
		       s.id AS submission_id,
		       COALESCE(s.score, s.auto_score) AS my_score,
		       s.submitted_at
		  FROM assignments a
		  JOIN class_members cm ON cm.class_id = a.class_id AND cm.user_id = ?
		  JOIN classes c       ON c.id = a.class_id
		  LEFT JOIN assignment_submissions s ON s.assignment_id = a.id AND s.user_id = ?
		 ORDER BY a.created_at DESC
		 LIMIT ?`, userId, userId, limit)
	return rows, err
}
