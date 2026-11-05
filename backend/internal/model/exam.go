package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ExamRow 考试表
type ExamRow struct {
	Id           int64        `db:"id" json:"id"`
	ClassId      int64        `db:"class_id" json:"classId"`
	Title        string       `db:"title" json:"title"`
	Description  string       `db:"description" json:"description"`
	StartAt      time.Time    `db:"start_at" json:"startAt"`
	EndAt        time.Time    `db:"end_at" json:"endAt"`
	DurationMin  int          `db:"duration_minutes" json:"durationMinutes"`
	TotalScore   int          `db:"total_score" json:"totalScore"`
	AllowLate    int          `db:"allow_late" json:"allowLate"`
	LateDeadline sql.NullTime `db:"late_deadline" json:"-"`
	TabLimit     int          `db:"tab_limit" json:"tabLimit"`
	CreatedBy    int64        `db:"created_by" json:"createdBy"`
	CreatedAt    time.Time    `db:"created_at" json:"createdAt"`
}

// MarshalJSON 把 late_deadline 转成前端可直接用的 ISO 字符串（空值输出 null）。
func (e ExamRow) MarshalJSON() ([]byte, error) {
	type Alias ExamRow
	var late *string
	if e.LateDeadline.Valid {
		s := e.LateDeadline.Time.Format(time.RFC3339)
		late = &s
	}
	return json.Marshal(&struct {
		LateDeadline *string `json:"lateDeadline"`
		Alias
	}{LateDeadline: late, Alias: Alias(e)})
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
	Id           int64         `db:"id" json:"id"`
	ExamId       int64         `db:"exam_id" json:"examId"`
	UserId       int64         `db:"user_id" json:"userId"`
	Username     string        `db:"username" json:"username"`
	Answers      string        `db:"answers" json:"answers"`
	Status       string        `db:"status" json:"status"`
	AutoScore    sql.NullInt64 `db:"auto_score" json:"-"`
	TeacherScore sql.NullInt64 `db:"teacher_score" json:"-"`
	Comment      string        `db:"comment" json:"comment"`
	GradedBy     int64         `db:"graded_by" json:"gradedBy"`
	GradedAt     sql.NullTime  `db:"graded_at" json:"-"`
	Violations   int           `db:"violations" json:"violations"`
	StartedAt    sql.NullTime  `db:"started_at" json:"startedAt"`
	SubmittedAt  time.Time     `db:"submitted_at" json:"submittedAt"`
}

// 交卷状态常量（与 V20 迁移里的注释保持一致）。
const (
	SubmitStatusStarted   = "started"   // 已开考，未交卷
	SubmitStatusSubmitted = "submitted" // 已交卷，锁定，不可再改
	SubmitStatusExpired   = "expired"   // 超时被系统自动收卷
)

// MarshalJSON 把 sql.NullInt64 / sql.NullTime 转成普通值，
// 避免前端收到 {"Int64":5,"Valid":true}。
func (s *ExamSubmissionRow) MarshalJSON() ([]byte, error) {
	type Alias ExamSubmissionRow
	var gradedAt *string
	if s.GradedAt.Valid {
		v := s.GradedAt.Time.Format(time.RFC3339)
		gradedAt = &v
	}
	return json.Marshal(&struct {
		AutoScore    *int64  `json:"autoScore"`
		TeacherScore *int64  `json:"teacherScore"`
		GradedAt     *string `json:"gradedAt"`
		*Alias
	}{
		AutoScore:    nullInt64Ptr(s.AutoScore),
		TeacherScore: nullInt64Ptr(s.TeacherScore),
		GradedAt:     gradedAt,
		Alias:        (*Alias)(s),
	})
}

func nullInt64Ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
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
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, late_deadline, tab_limit, created_by, created_at FROM exams WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (m *ExamModel) ListExamsByClass(classId int64) ([]*ExamRow, error) {
	var rows []*ExamRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, late_deadline, tab_limit, created_by, created_at FROM exams WHERE class_id=? ORDER BY start_at DESC", classId)
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
		"SELECT id, class_id, title, description, start_at, end_at, duration_minutes, total_score, allow_late, late_deadline, tab_limit, created_by, created_at FROM exams WHERE class_id=? ORDER BY start_at DESC LIMIT ? OFFSET ?",
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
	var late interface{}
	if e.LateDeadline.Valid {
		late = e.LateDeadline.Time
	}
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE exams SET title=?, description=?, start_at=?, end_at=?, duration_minutes=?, total_score=?, allow_late=?, late_deadline=?, tab_limit=? WHERE id=?`,
		e.Title, e.Description, e.StartAt, e.EndAt, e.DurationMin, e.TotalScore, e.AllowLate, late, e.TabLimit, e.Id)
	return err
}

// MaxOrderNum 查某考试下当前最大 order_num，新题追加时用。
func (m *ExamModel) MaxOrderNum(examId int64) (int, error) {
	var n int
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COALESCE(MAX(order_num),0) FROM exam_questions WHERE exam_id=?", examId)
	return n, err
}

// SubmitOnce 交卷（一次性，禁止覆盖）。
//
// 改造前是裸 UpsertSubmission：学生交完卷、查到答案后还能再交一次刷分。
// 这里改成"条件更新"：只有 status='started' 的行会被写入，
// 已经 submitted / expired 的行更新 0 行，调用方据此返回 409。
//
// 没开考直接交卷（没有行）时插入一条提交记录，兼容"前端没走 StartExam"的老流程。
// 返回 true 表示本次交卷成功落库。
func (m *ExamModel) SubmitOnce(s *ExamSubmissionRow) (bool, error) {
	ctx := context.Background()
	res, err := m.conn.ExecCtx(ctx,
		`UPDATE exam_submissions
		    SET answers=?, auto_score=?, status='submitted', submitted_at=NOW()
		  WHERE exam_id=? AND user_id=? AND status='started'`,
		s.Answers, s.AutoScore, s.ExamId, s.UserId)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}

	// 更新 0 行：要么没开考记录，要么已经交过卷。先查一眼区分这两种情况。
	cur, qerr := m.GetMySubmission(s.ExamId, s.UserId)
	if qerr != nil && qerr != sql.ErrNoRows {
		return false, qerr
	}
	if cur != nil {
		// 已存在且不是 started → 重复交卷
		return false, nil
	}

	// 不存在：允许直接交卷（学生没点"开始考试"但答完了）
	_, ierr := m.conn.ExecCtx(ctx,
		`INSERT INTO exam_submissions (exam_id, user_id, answers, auto_score, status, submitted_at)
		 VALUES (?,?,?,?,'submitted',NOW())`,
		s.ExamId, s.UserId, s.Answers, s.AutoScore)
	if ierr != nil {
		return false, ierr
	}
	return true, nil
}

// SubmitOnceTx 交卷的事务版本：在传入的 session 上执行，供"交卷 + 写成绩册"
// 的跨表事务（examlogic.Submit）复用，保证原子。逻辑与 SubmitOnce 完全一致。
func (m *ExamModel) SubmitOnceTx(ctx context.Context, session sqlx.Session, s *ExamSubmissionRow) (bool, error) {
	res, err := session.ExecCtx(ctx,
		`UPDATE exam_submissions
		    SET answers=?, auto_score=?, status='submitted', submitted_at=NOW()
		  WHERE exam_id=? AND user_id=? AND status='started'`,
		s.Answers, s.AutoScore, s.ExamId, s.UserId)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}

	// 更新 0 行：要么没开考记录，要么已经交过卷。先查一眼区分这两种情况。
	cur, qerr := m.GetMySubmission(s.ExamId, s.UserId)
	if qerr != nil && qerr != sql.ErrNoRows {
		return false, qerr
	}
	if cur != nil {
		return false, nil
	}

	// 不存在：允许直接交卷（学生没点"开始考试"但答完了）
	_, ierr := session.ExecCtx(ctx,
		`INSERT INTO exam_submissions (exam_id, user_id, answers, auto_score, status, submitted_at)
		 VALUES (?,?,?,?,'submitted',NOW())`,
		s.ExamId, s.UserId, s.Answers, s.AutoScore)
	if ierr != nil {
		return false, ierr
	}
	return true, nil
}

// GetMySubmission 取某学生某场考试的提交记录；不存在返回 (nil, nil)。
func (m *ExamModel) GetMySubmission(examId, userId int64) (*ExamSubmissionRow, error) {
	var s ExamSubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		`SELECT s.id, s.exam_id, s.user_id, u.username, s.answers, s.status, s.auto_score, s.teacher_score,
		        s.comment, s.graded_by, s.graded_at, s.violations, s.started_at, s.submitted_at
		   FROM exam_submissions s JOIN users u ON s.user_id=u.id
		  WHERE s.exam_id=? AND s.user_id=?`, examId, userId)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSubmission 按 id 取一条提交记录（老师批改前校验所属班级用）。
func (m *ExamModel) GetSubmission(submissionId int64) (*ExamSubmissionRow, error) {
	var s ExamSubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		`SELECT s.id, s.exam_id, s.user_id, u.username, s.answers, s.status, s.auto_score, s.teacher_score,
		        s.comment, s.graded_by, s.graded_at, s.violations, s.started_at, s.submitted_at
		   FROM exam_submissions s JOIN users u ON s.user_id=u.id
		  WHERE s.id=?`, submissionId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSubmissions 某场考试的全部提交（老师端）。
func (m *ExamModel) ListSubmissions(examId int64) ([]*ExamSubmissionRow, error) {
	var rows []*ExamSubmissionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT s.id, s.exam_id, s.user_id, u.username, s.answers, s.status, s.auto_score, s.teacher_score,
		        s.comment, s.graded_by, s.graded_at, s.violations, s.started_at, s.submitted_at
		   FROM exam_submissions s JOIN users u ON s.user_id=u.id
		  WHERE s.exam_id=? ORDER BY s.submitted_at DESC`, examId)
	return rows, err
}

// GradeSubmission 老师手批：写分数 + 批改人 + 批改时间。
func (m *ExamModel) GradeSubmission(submissionId int64, teacherScore int, comment string, gradedBy int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE exam_submissions SET teacher_score=?, comment=?, graded_by=?, graded_at=NOW() WHERE id=?",
		teacherScore, comment, gradedBy, submissionId)
	return err
}

// StartSubmission 学生开考：写一行 submission，started_at=now，status='started'。
// 唯一键 (exam_id, user_id)：重复调用不覆盖 started_at（用 INSERT IGNORE），
// 学生可以反复点"开始"但只记第一次的时间——这也保证了时长不被重置。
func (m *ExamModel) StartSubmission(examId, userId int64, startedAt interface{}) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO exam_submissions (exam_id, user_id, started_at, status) VALUES (?,?,?,'started')",
		examId, userId, startedAt)
	return err
}

// OverdueSubmission 一行"超时未交卷"的记录：把考试时间窗口一起带出来，
// 供后台自动收卷任务在 Go 里用统一规则判定（规则只在 examrules 里有一份）。
type OverdueSubmission struct {
	ExamId       int64        `db:"exam_id"`
	UserId       int64        `db:"user_id"`
	SubmissionId int64        `db:"submission_id"`
	StartedAt    sql.NullTime `db:"started_at"`
	StartAt      time.Time    `db:"start_at"`
	EndAt        time.Time    `db:"end_at"`
	DurationMin  int          `db:"duration_minutes"`
	AllowLate    int          `db:"allow_late"`
	LateDeadline sql.NullTime `db:"late_deadline"`
	// Answers 最后一次草稿。扫描时一起带出来，收卷时不用再按 submission 回查一次。
	Answers string `db:"answers"`
	// 以下字段用于超时收卷时按草稿判分、写成绩册
	ClassId    int64  `db:"class_id"`
	Title      string `db:"title"`
	TotalScore int    `db:"total_score"`
}

// ListStartedSubmissions 列出所有"考试中"的记录（后台自动收卷扫描用）。
//
// 走 idx_status_started(status, started_at)：只扫未交卷的行，
// 而不是每次全表扫 exam_submissions。
func (m *ExamModel) ListStartedSubmissions() ([]*OverdueSubmission, error) {
	var rows []*OverdueSubmission
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT s.exam_id, s.user_id, s.id AS submission_id, s.started_at, s.answers,
		        e.start_at, e.end_at, e.duration_minutes, e.allow_late, e.late_deadline,
		        e.class_id, e.title, e.total_score
		   FROM exam_submissions s JOIN exams e ON e.id = s.exam_id
		  WHERE s.status = 'started'`)
	return rows, err
}

// ListQuestionsByExamIds 批量取多场考试的题目，按 exam_id 分组返回。
//
// 自动收卷一轮可能同时处理几十份超时答卷，其中很多属于同一场考试。
// 逐份调用 ListQuestions 会把同一份试卷重复查 N 次；这里一次 IN 查询取回全部，
// 收卷循环里只在内存里查表。
func (m *ExamModel) ListQuestionsByExamIds(examIds []int64) (map[int64][]*ExamQuestionRow, error) {
	out := make(map[int64][]*ExamQuestionRow, len(examIds))
	if len(examIds) == 0 {
		return out, nil
	}
	holders := strings.TrimSuffix(strings.Repeat("?,", len(examIds)), ",")
	args := make([]interface{}, 0, len(examIds))
	for _, id := range examIds {
		args = append(args, id)
	}
	var rows []*ExamQuestionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT id, exam_id, type, stem, options, answer, score, order_num, required, created_at
		   FROM exam_questions
		  WHERE exam_id IN (`+holders+`)
		  ORDER BY exam_id, order_num ASC, id ASC`, args...)
	if err != nil {
		return out, err
	}
	for _, q := range rows {
		out[q.ExamId] = append(out[q.ExamId], q)
	}
	return out, nil
}

// ExpireSubmission 超时强制收卷：写入 status='expired' 并按最后一次草稿记自动分。
//
// 与旧实现的区别：旧版只把状态改成 expired、分数留空，学生答了但没点交卷就是 0 分；
// 现在前端会定时上报草稿，收卷时按草稿判分，网络中断不再等于白考。
//
// 返回 true 表示本次确实完成了状态迁移；false 表示这期间学生已经交卷
// （条件更新 0 行），调用方不能再拿草稿覆盖他的成绩。
func (m *ExamModel) ExpireSubmission(submissionId int64, autoScore int) (bool, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		`UPDATE exam_submissions
		    SET status='expired', submitted_at=NOW(), auto_score=?,
		        answers = CASE WHEN answers IS NULL THEN '{}' ELSE answers END
		  WHERE id=? AND status='started'`, autoScore, submissionId)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveDraft 保存作答草稿（考试进行中）。
// 只有 status='started' 的行会被更新；已交卷/已超时返回 false，前端据此提示"已锁定"。
func (m *ExamModel) SaveDraft(examId, userId int64, answers string) (bool, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		`UPDATE exam_submissions SET answers=? WHERE exam_id=? AND user_id=? AND status='started'`,
		answers, examId, userId)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// AddViolation 记录一次违纪（切屏/离开全屏）。前端上报，服务端累加，
// 不做本地信任：老师端看到的是服务端计数。
func (m *ExamModel) AddViolation(examId, userId int64) (int, error) {
	ctx := context.Background()
	if _, err := m.conn.ExecCtx(ctx,
		"UPDATE exam_submissions SET violations = violations + 1 WHERE exam_id=? AND user_id=?",
		examId, userId); err != nil {
		return 0, err
	}
	var n int
	err := m.conn.QueryRowCtx(ctx, &n,
		"SELECT violations FROM exam_submissions WHERE exam_id=? AND user_id=?", examId, userId)
	return n, err
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
