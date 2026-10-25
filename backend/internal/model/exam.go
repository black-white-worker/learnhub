package model

import (
	"context"
	"database/sql"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ExamRow 考试表
type ExamRow struct {
	Id             int64        `db:"id" json:"id"`
	ClassId        int64        `db:"class_id" json:"classId"`
	Title          string       `db:"title" json:"title"`
	Description    string       `db:"description" json:"description"`
	StartAt        time.Time    `db:"start_at" json:"startAt"`
	EndAt          time.Time    `db:"end_at" json:"endAt"`
	DurationMin    int          `db:"duration_minutes" json:"durationMinutes"`
	TotalScore     int          `db:"total_score" json:"totalScore"`
	CreatedBy      int64        `db:"created_by" json:"createdBy"`
	CreatedAt      time.Time    `db:"created_at" json:"createdAt"`
}

// ExamQuestionRow 考试题目
type ExamQuestionRow struct {
	Id         int64  `db:"id" json:"id"`
	ExamId     int64  `db:"exam_id" json:"examId"`
	Type       string `db:"type" json:"type"`
	Stem       string `db:"stem" json:"stem"`
	Options    string `db:"options" json:"options"`
	Answer     string `db:"answer" json:"-"` // 不返回给学生
	Score      int    `db:"score" json:"score"`
}

// ExamSubmissionRow 考试提交记录
type ExamSubmissionRow struct {
	Id           int64          `db:"id" json:"id"`
	ExamId       int64          `db:"exam_id" json:"examId"`
	UserId       int64          `db:"user_id" json:"userId"`
	Username     string         `db:"username" json:"username"`
	Answers      string         `db:"answers" json:"answers"`
	AutoScore    sql.NullInt64  `db:"auto_score" json:"autoScore"`
	TeacherScore sql.NullInt64  `db:"teacher_score" json:"teacherScore"`
	Comment      string         `db:"comment" json:"comment"`
	StartedAt    sql.NullTime   `db:"started_at" json:"startedAt"`
	SubmittedAt  time.Time      `db:"submitted_at" json:"submittedAt"`
}

type ExamModel struct {
	conn sqlx.SqlConn
}

func NewExamModel(conn sqlx.SqlConn) *ExamModel {
	return &ExamModel{conn: conn}
}

func (m *ExamModel) InsertExam(e *ExamRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO exams (class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by) VALUES (?,?,?,?,?,?,?,?)",
		e.ClassId, e.Title, e.Description, e.StartAt, e.EndAt, e.DurationMin, e.TotalScore, e.CreatedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ExamModel) GetExam(id int64) (*ExamRow, error) {
	var e ExamRow
	err := m.conn.QueryRowCtx(context.Background(), &e,
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by, created_at FROM exams WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (m *ExamModel) ListExamsByClass(classId int64) ([]*ExamRow, error) {
	var rows []*ExamRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by, created_at FROM exams WHERE class_id=? ORDER BY start_at DESC", classId)
	return rows, err
}

func (m *ExamModel) InsertQuestion(q *ExamQuestionRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO exam_questions (exam_id, type, stem, options, answer, score) VALUES (?,?,?,?,?,?)",
		q.ExamId, q.Type, q.Stem, q.Options, q.Answer, q.Score)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ExamModel) ListQuestions(examId int64) ([]*ExamQuestionRow, error) {
	var rows []*ExamQuestionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, exam_id, type, stem, options, answer, score FROM exam_questions WHERE exam_id=? ORDER BY id", examId)
	return rows, err
}

// UpsertSubmission 学生交卷，唯一键 (exam_id, user_id)
func (m *ExamModel) UpsertSubmission(s *ExamSubmissionRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO exam_submissions (exam_id, user_id, answers, auto_score, started_at)
		 VALUES (?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE answers=VALUES(answers), auto_score=VALUES(auto_score), submitted_at=NOW()`,
		s.ExamId, s.UserId, s.Answers, s.AutoScore, s.StartedAt)
	return err
}

func (m *ExamModel) GetMySubmission(examId, userId int64) (*ExamSubmissionRow, error) {
	var s ExamSubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		`SELECT s.id, s.exam_id, s.user_id, u.username, s.answers, s.auto_score, s.teacher_score, s.comment, s.started_at, s.submitted_at
		 FROM exam_submissions s JOIN users u ON s.user_id=u.id
		 WHERE s.exam_id=? AND s.user_id=?`, examId, userId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *ExamModel) ListSubmissions(examId int64) ([]*ExamSubmissionRow, error) {
	var rows []*ExamSubmissionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT s.id, s.exam_id, s.user_id, u.username, s.answers, s.auto_score, s.teacher_score, s.comment, s.started_at, s.submitted_at
		 FROM exam_submissions s JOIN users u ON s.user_id=u.id
		 WHERE s.exam_id=? ORDER BY s.submitted_at DESC`, examId)
	return rows, err
}

func (m *ExamModel) GradeSubmission(submissionId int64, teacherScore int, comment string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE exam_submissions SET teacher_score=?, comment=? WHERE id=?", teacherScore, comment, submissionId)
	return err
}
