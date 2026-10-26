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
	AllowLate      int          `db:"allow_late" json:"allowLate"`
	TabLimit       int          `db:"tab_limit" json:"tabLimit"`
	CreatedBy      int64        `db:"created_by" json:"createdBy"`
	CreatedAt      time.Time    `db:"created_at" json:"createdAt"`
}

// ExamQuestionRow 考试题目
type ExamQuestionRow struct {
	Id        int64  `db:"id" json:"id"`
	ExamId    int64  `db:"exam_id" json:"examId"`
	Type      string `db:"type" json:"type"`
	Stem      string `db:"stem" json:"stem"`
	Options   string `db:"options" json:"options"`
	Answer    string `db:"answer" json:"answer"`
	Score     int    `db:"score" json:"score"`
	OrderNum  int    `db:"order_num" json:"orderNum"`
	Required  int    `db:"required" json:"required"`
	CreatedAt string `db:"created_at" json:"createdAt"`
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
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, tab_limit, created_by, created_at FROM exams WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (m *ExamModel) ListExamsByClass(classId int64) ([]*ExamRow, error) {
	var rows []*ExamRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, tab_limit, created_by, created_at FROM exams WHERE class_id=? ORDER BY start_at DESC", classId)
	return rows, err
}

// ListExamsByClassPaged 分页：返回本页数据和总数。
func (m *ExamModel) ListExamsByClassPaged(classId int64, page, size int) ([]*ExamRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	var total int64
	if err := m.conn.QueryRowCtx(context.Background(), &total,
		"SELECT COUNT(*) FROM exams WHERE class_id=?", classId); err != nil {
		return nil, 0, err
	}
	var rows []*ExamRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, tab_limit, created_by, created_at FROM exams WHERE class_id=? ORDER BY start_at DESC LIMIT ? OFFSET ?",
		classId, size, offset)
	return rows, total, err
}

func (m *ExamModel) InsertQuestion(q *ExamQuestionRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO exam_questions (exam_id, type, stem, options, answer, score, order_num, required) VALUES (?,?,?,?,?,?,?,?)",
		q.ExamId, q.Type, q.Stem, q.Options, q.Answer, q.Score, q.OrderNum, q.Required)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ExamModel) ListQuestions(examId int64) ([]*ExamQuestionRow, error) {
	var rows []*ExamQuestionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, exam_id, type, stem, options, answer, score, order_num, required, created_at FROM exam_questions WHERE exam_id=? ORDER BY order_num ASC, id ASC", examId)
	return rows, err
}

// UpdateQuestion 更新单题：题干/选项/答案/分值/必答。只更新非零字段。
func (m *ExamModel) UpdateQuestion(q *ExamQuestionRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE exam_questions SET type=?, stem=?, options=?, answer=?, score=?, required=? WHERE id=? AND exam_id=?`,
		q.Type, q.Stem, q.Options, q.Answer, q.Score, q.Required, q.Id, q.ExamId)
	return err
}

// DeleteQuestion 删题。
func (m *ExamModel) DeleteQuestion(qid int64) error {
	_, err := m.conn.ExecCtx(context.Background(), "DELETE FROM exam_questions WHERE id=?", qid)
	return err
}

// GetQuestion 取单题。
func (m *ExamModel) GetQuestion(qid int64) (*ExamQuestionRow, error) {
	var q ExamQuestionRow
	err := m.conn.QueryRowCtx(context.Background(), &q,
		"SELECT id, exam_id, type, stem, options, answer, score, order_num, required, created_at FROM exam_questions WHERE id=?", qid)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// SortQuestions 批量改排序号：事务里按 order 数组逐个 UPDATE order_num。
// order 元素是题目 id，数组下标即新的顺序（从 1 开始）。
func (m *ExamModel) SortQuestions(examId int64, order []int64) error {
	return m.conn.TransactCtx(context.Background(), func(ctx context.Context, tx sqlx.Session) error {
		for i, qid := range order {
			if _, err := tx.ExecCtx(ctx,
				"UPDATE exam_questions SET order_num=? WHERE id=? AND exam_id=?", i+1, qid, examId); err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateExam 更新考试基本信息。
func (m *ExamModel) UpdateExam(e *ExamRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE exams SET title=?, description=?, start_at=?, end_at=?, duration_minutes=?, total_score=?, allow_late=?, tab_limit=? WHERE id=?`,
		e.Title, e.Description, e.StartAt, e.EndAt, e.DurationMin, e.TotalScore, e.AllowLate, e.TabLimit, e.Id)
	return err
}

// MaxOrderNum 查某考试下当前最大 order_num，新题追加时用。
func (m *ExamModel) MaxOrderNum(examId int64) (int, error) {
	var n int
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COALESCE(MAX(order_num),0) FROM exam_questions WHERE exam_id=?", examId)
	return n, err
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

// StartSubmission 学生开考：写一行 submission，started_at=now。
// 唯一键 (exam_id, user_id)：重复调用不覆盖 started_at（用 INSERT IGNORE），
// 学生可以反复点"开始"但只记第一次的时间。
func (m *ExamModel) StartSubmission(examId, userId int64, startedAt interface{}) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO exam_submissions (exam_id, user_id, started_at) VALUES (?,?,?)",
		examId, userId, startedAt)
	return err
}

// BatchInsertQuestions 事务批量插入题目，返回新题 id 列表。
// 教师手动组卷：一次把整份试卷的题目塞进库。
func (m *ExamModel) BatchInsertQuestions(qs []*ExamQuestionRow) ([]int64, error) {
	if len(qs) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(qs))
	err := m.conn.TransactCtx(context.Background(), func(ctx context.Context, tx sqlx.Session) error {
		for _, q := range qs {
			res, err := tx.ExecCtx(ctx,
				"INSERT INTO exam_questions (exam_id, type, stem, options, answer, score) VALUES (?,?,?,?,?,?)",
				q.ExamId, q.Type, q.Stem, q.Options, q.Answer, q.Score)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
