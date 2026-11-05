package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// ClassModel 班级模块数据访问层
type ClassModel struct {
	conn sqlx.SqlConn
}

func NewClassModel(conn sqlx.SqlConn) *ClassModel {
	return &ClassModel{conn: conn}
}

// ---- classes 表 ----

type ClassRow struct {
	Id            int64     `db:"id" json:"id"`
	Name          string    `db:"name" json:"name"`
	Grade         string    `db:"grade" json:"grade"`
	Major         string    `db:"major" json:"major"`
	HeadTeacherId int64     `db:"head_teacher_id" json:"headTeacherId"`
	CreatedBy     string    `db:"created_by" json:"createdBy"`
	CreatedAt     time.Time `db:"created_at" json:"createdAt"`
}

func (m *ClassModel) InsertClass(name, grade, major, createdBy string, headTeacherId int64) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO classes (name, grade, major, head_teacher_id, created_by) VALUES (?,?,?,?,?)",
		name, grade, major, headTeacherId, createdBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ClassModel) GetClass(id int64) (*ClassRow, error) {
	var c ClassRow
	err := m.conn.QueryRowCtx(context.Background(), &c,
		"SELECT id, name, grade, major, head_teacher_id, created_by, created_at FROM classes WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListClassesByUser 返回用户加入的所有班级（学生看自己班，老师看自己带的班）
func (m *ClassModel) ListClassesByUser(userId int64) ([]*ClassRow, error) {
	var rows []*ClassRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT c.id, c.name, c.grade, c.major, c.head_teacher_id, c.created_by, c.created_at
		 FROM classes c JOIN class_members cm ON c.id=cm.class_id
		 WHERE cm.user_id=? ORDER BY c.id DESC`, userId)
	return rows, err
}

// ListAllClasses admin 用，返回所有班
func (m *ClassModel) ListAllClasses() ([]*ClassRow, error) {
	var rows []*ClassRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, name, grade, major, head_teacher_id, created_by, created_at FROM classes ORDER BY id DESC")
	return rows, err
}

func (m *ClassModel) DeleteClass(id int64) error {
	ctx := context.Background()
	return m.conn.TransactCtx(ctx, func(ctx context.Context, s sqlx.Session) error {
		if _, err := s.ExecCtx(ctx, "DELETE FROM class_members WHERE class_id=?", id); err != nil {
			return err
		}
		if _, err := s.ExecCtx(ctx, "DELETE FROM class_courses WHERE class_id=?", id); err != nil {
			return err
		}
		if _, err := s.ExecCtx(ctx, "DELETE FROM announcements WHERE class_id=?", id); err != nil {
			return err
		}
		if _, err := s.ExecCtx(ctx, "DELETE FROM classes WHERE id=?", id); err != nil {
			return err
		}
		return nil
	})
}

// ---- class_members 表 ----

type ClassMemberRow struct {
	Id       int64  `db:"id" json:"id"`
	ClassId  int64  `db:"class_id" json:"classId"`
	UserId   int64  `db:"user_id" json:"userId"`
	Role     string `db:"role" json:"role"`
	Username string `db:"username" json:"username"`
	Nickname string `db:"nickname" json:"nickname"`
}

func (m *ClassModel) AddMember(classId, userId int64, role string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO class_members (class_id, user_id, role) VALUES (?,?,?)",
		classId, userId, role)
	return err
}

func (m *ClassModel) RemoveMember(classId, userId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"DELETE FROM class_members WHERE class_id=? AND user_id=?", classId, userId)
	return err
}

// GetUserClassRole 查用户在某班的角色（teacher/student/空=不在班）
func (m *ClassModel) GetUserClassRole(classId, userId int64) (string, error) {
	var role string
	err := m.conn.QueryRowCtx(context.Background(), &role,
		"SELECT role FROM class_members WHERE class_id=? AND user_id=?", classId, userId)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

func (m *ClassModel) ListMembers(classId int64) ([]*ClassMemberRow, error) {
	var rows []*ClassMemberRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT cm.id, cm.class_id, cm.user_id, cm.role, u.username, u.nickname
		 FROM class_members cm JOIN users u ON cm.user_id=u.id
		 WHERE cm.class_id=? ORDER BY cm.role DESC, cm.id`, classId)
	return rows, err
}

// ---- class_courses 表 ----

func (m *ClassModel) BindCourse(classId, courseId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO class_courses (class_id, course_id) VALUES (?,?)", classId, courseId)
	return err
}

func (m *ClassModel) UnbindCourse(classId, courseId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"DELETE FROM class_courses WHERE class_id=? AND course_id=?", classId, courseId)
	return err
}

// CourseEnrollmentStats 课程的教学范围：
//   - bound 这门课被多少个班级绑定（0 = 独立公开课）
//   - mine  当前用户加入了其中多少个班级
//
// 用于媒体访问控制（见 course/media_access.go）：一门课只要被班级绑定，
// 就要求访问者是其中某个班的成员，否则只有创建者/管理员能看。
func (m *ClassModel) CourseEnrollmentStats(courseId, userId int64) (bound, mine int, err error) {
	var row struct {
		Bound int `db:"bound"`
		Mine  int `db:"mine"`
	}
	err = m.conn.QueryRowCtx(context.Background(), &row, `
		SELECT
		  (SELECT COUNT(*) FROM class_courses WHERE course_id = ?) AS bound,
		  (SELECT COUNT(*) FROM class_courses cc
		     JOIN class_members cm ON cm.class_id = cc.class_id
		    WHERE cc.course_id = ? AND cm.user_id = ?) AS mine`,
		courseId, courseId, userId)
	if err != nil {
		return 0, 0, err
	}
	return row.Bound, row.Mine, nil
}

func (m *ClassModel) ListClassCourseIds(classId int64) ([]int64, error) {
	var ids []int64
	err := m.conn.QueryRowsCtx(context.Background(), &ids,
		"SELECT course_id FROM class_courses WHERE class_id=?", classId)
	return ids, err
}

// ---- class_invitations 表 ----

type InvitationRow struct {
	Id        int64        `db:"id" json:"id"`
	ClassId   int64        `db:"class_id" json:"classId"`
	Code      string       `db:"code"`
	ExpiresAt sql.NullTime `db:"expires_at"`
	MaxUses   int          `db:"max_uses"`
	UsedCount int          `db:"used_count"`
}

func (m *ClassModel) CreateInvitation(classId int64, code string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO class_invitations (class_id, code) VALUES (?,?)", classId, code)
	return err
}

func (m *ClassModel) GetInvitationByCode(code string) (*InvitationRow, error) {
	var inv InvitationRow
	err := m.conn.QueryRowCtx(context.Background(), &inv,
		"SELECT id, class_id, code, expires_at, max_uses, used_count FROM class_invitations WHERE code=?", code)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func (m *ClassModel) IncInvitationUse(id int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE class_invitations SET used_count=used_count+1 WHERE id=?", id)
	return err
}

func (m *ClassModel) ListInvitations(classId int64) ([]*InvitationRow, error) {
	var rows []*InvitationRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, code, expires_at, max_uses, used_count FROM class_invitations WHERE class_id=?", classId)
	return rows, err
}

// ---- announcements 公告 ----

type AnnouncementRow struct {
	Id          int64     `db:"id" json:"id"`
	ClassId     int64     `db:"class_id" json:"classId"`
	Title       string    `db:"title" json:"title"`
	Content     string    `db:"content" json:"content"`
	PublisherId int64     `db:"publisher_id" json:"publisherId"`
	IsTop       bool      `db:"is_top" json:"isTop"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

func (m *ClassModel) InsertAnnouncement(a *AnnouncementRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO announcements (class_id, title, content, publisher_id, is_top) VALUES (?,?,?,?,?)",
		a.ClassId, a.Title, a.Content, a.PublisherId, a.IsTop)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ClassModel) ListAnnouncements(classId int64) ([]*AnnouncementRow, error) {
	var rows []*AnnouncementRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, content, publisher_id, is_top, created_at FROM announcements WHERE class_id=? ORDER BY is_top DESC, created_at DESC", classId)
	return rows, err
}

// ListAnnouncementsPaged 公告分页。
func (m *ClassModel) ListAnnouncementsPaged(classId int64, page, size int) ([]*AnnouncementRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	var total int64
	if err := m.conn.QueryRowCtx(context.Background(), &total,
		"SELECT COUNT(*) FROM announcements WHERE class_id=?", classId); err != nil {
		return nil, 0, err
	}
	var rows []*AnnouncementRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, content, publisher_id, is_top, created_at FROM announcements WHERE class_id=? ORDER BY is_top DESC, created_at DESC LIMIT ? OFFSET ?",
		classId, size, offset)
	return rows, total, err
}

func (m *ClassModel) DeleteAnnouncement(id int64) error {
	_, err := m.conn.ExecCtx(context.Background(), "DELETE FROM announcements WHERE id=?", id)
	return err
}

// GetAnnouncement 按 id 取单条公告（删除前鉴权用，不存在返回 sql.ErrNoRows）
func (m *ClassModel) GetAnnouncement(id int64) (*AnnouncementRow, error) {
	var a AnnouncementRow
	err := m.conn.QueryRowCtx(context.Background(), &a,
		"SELECT id, class_id, title, content, publisher_id, is_top, created_at FROM announcements WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- assignments 作业 ----

type AssignmentRow struct {
	Id           int64        `db:"id" json:"id"`
	ClassId      int64        `db:"class_id" json:"classId"`
	CourseId     int64        `db:"course_id" json:"courseId"`
	Title        string       `db:"title" json:"title"`
	Description  string       `db:"description" json:"description"`
	Deadline     sql.NullTime `db:"deadline" json:"-"` // 用 MarshalJSON 自定义输出，避免暴露 {Time,Valid} 内部结构
	TotalScore   int          `db:"total_score" json:"totalScore"`
	AllowLate    int          `db:"allow_late" json:"allowLate"`
	LateDeadline sql.NullTime `db:"late_deadline" json:"-"`
	PublisherId  int64        `db:"publisher_id" json:"publisherId"`
	CreatedAt    time.Time    `db:"created_at" json:"createdAt"`
}

// MarshalJSON 自定义作业 JSON 输出：
// Deadline 是 sql.NullTime，默认会序列化成 {"Time":"...","Valid":false} 这种内部结构，
// 前端直接 {{ a.deadline }} 渲染就会出现截图里那串原始 JSON。
// 这里统一输出成 "2006-01-02 15:04" 字符串，没设截止时间则输出 null，
// 前端用 {{ a.deadline || '无截止时间' }} 即可正常显示。
func (a *AssignmentRow) MarshalJSON() ([]byte, error) {
	type Alias AssignmentRow // 别名避免递归调用本方法
	dl := ""
	if a.Deadline.Valid {
		dl = a.Deadline.Time.Format("2006-01-02 15:04")
	}
	ldl := ""
	if a.LateDeadline.Valid {
		ldl = a.LateDeadline.Time.Format("2006-01-02 15:04")
	}
	return json.Marshal(&struct {
		Deadline     string `json:"deadline"`
		LateDeadline string `json:"lateDeadline"`
		*Alias
	}{
		Deadline:     dl,
		LateDeadline: ldl,
		Alias:        (*Alias)(a),
	})
}

func (m *ClassModel) InsertAssignment(a *AssignmentRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO assignments (class_id, course_id, title, description, deadline, total_score, publisher_id) VALUES (?,?,?,?,?,?,?)",
		a.ClassId, a.CourseId, a.Title, a.Description, a.Deadline, a.TotalScore, a.PublisherId)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ClassModel) ListAssignments(classId int64) ([]*AssignmentRow, error) {
	var rows []*AssignmentRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, course_id, title, description, deadline, total_score, allow_late, late_deadline, publisher_id, created_at FROM assignments WHERE class_id=? ORDER BY created_at DESC", classId)
	return rows, err
}

func (m *ClassModel) GetAssignment(id int64) (*AssignmentRow, error) {
	var a AssignmentRow
	err := m.conn.QueryRowCtx(context.Background(), &a,
		"SELECT id, class_id, course_id, title, description, deadline, total_score, allow_late, late_deadline, publisher_id, created_at FROM assignments WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- assignment_submissions 作业提交 ----

type SubmissionRow struct {
	Id           int64  `db:"id" json:"id"`
	AssignmentId int64  `db:"assignment_id"`
	UserId       int64  `db:"user_id" json:"userId"`
	Content      string `db:"content" json:"content"`
	// Answers 客观题作答 JSON，V11 加列。学生提交时由 logic 层写入。
	Answers     string        `db:"answers" json:"answers"`
	FileUrl     string        `db:"file_url" json:"fileUrl"`
	Score       sql.NullInt64 `db:"score" json:"-"`      // 老师手打分，MarshalJSON 输出数字或 null
	AutoScore   sql.NullInt64 `db:"auto_score" json:"-"` // 客观题自动判分，MarshalJSON 输出数字或 null
	Comment     string        `db:"comment" json:"comment"`
	SubmittedAt time.Time     `db:"submitted_at" json:"submittedAt"`
}

// MarshalJSON 自定义：把 Score/AutoScore 两个 sql.NullInt64 输出成数字或 null，
// 避免默认序列化成 {"Int":10,"Valid":true} 这种内部结构让前端拿不到数字。
func (s *SubmissionRow) MarshalJSON() ([]byte, error) {
	type Alias SubmissionRow
	return json.Marshal(&struct {
		Score     *int `json:"score"`
		AutoScore *int `json:"autoScore"`
		*Alias
	}{
		Score:     nullIntToPtr(s.Score),
		AutoScore: nullIntToPtr(s.AutoScore),
		Alias:     (*Alias)(s),
	})
}

func nullIntToPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func (m *ClassModel) UpsertSubmission(s *SubmissionRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO assignment_submissions (assignment_id, user_id, content, file_url)
		 VALUES (?,?,?,?)
		 ON DUPLICATE KEY UPDATE content=VALUES(content), file_url=VALUES(file_url), submitted_at=NOW()`,
		s.AssignmentId, s.UserId, s.Content, s.FileUrl)
	return err
}

func (m *ClassModel) GradeSubmission(submissionId int64, score int, comment string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE assignment_submissions SET score=?, comment=?, graded_at=NOW() WHERE id=?", score, comment, submissionId)
	return err
}

// GetSubmission 按提交 id 取单条提交记录（批改前鉴权用）
func (m *ClassModel) GetSubmission(submissionId int64) (*SubmissionRow, error) {
	var s SubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, assignment_id, user_id, content, answers, file_url, score, auto_score, comment, submitted_at FROM assignment_submissions WHERE id=?",
		submissionId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (m *ClassModel) ListSubmissions(assignmentId int64) ([]*SubmissionRow, error) {
	var rows []*SubmissionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, assignment_id, user_id, content, answers, file_url, score, auto_score, comment, submitted_at FROM assignment_submissions WHERE assignment_id=? ORDER BY submitted_at DESC", assignmentId)
	return rows, err
}

func (m *ClassModel) GetMySubmission(assignmentId, userId int64) (*SubmissionRow, error) {
	var s SubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, assignment_id, user_id, content, answers, file_url, score, auto_score, comment, submitted_at FROM assignment_submissions WHERE assignment_id=? AND user_id=?",
		assignmentId, userId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ---- discussions 讨论 ----

type DiscussionRow struct {
	Id          int64     `db:"id" json:"id"`
	ClassId     int64     `db:"class_id" json:"classId"`
	UserId      int64     `db:"user_id" json:"userId"`
	Title       string    `db:"title" json:"title"`
	Content     string    `db:"content" json:"content"`
	LastReplyAt time.Time `db:"last_reply_at" json:"lastReplyAt"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
}

func (m *ClassModel) InsertDiscussion(d *DiscussionRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO discussions (class_id, user_id, title, content) VALUES (?,?,?,?)",
		d.ClassId, d.UserId, d.Title, d.Content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ClassModel) ListDiscussions(classId int64, limit, offset int) ([]*DiscussionRow, error) {
	var rows []*DiscussionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, user_id, title, content, last_reply_at, created_at FROM discussions WHERE class_id=? ORDER BY last_reply_at DESC LIMIT ? OFFSET ?",
		classId, limit, offset)
	return rows, err
}

func (m *ClassModel) GetDiscussion(id int64) (*DiscussionRow, error) {
	var d DiscussionRow
	err := m.conn.QueryRowCtx(context.Background(), &d,
		"SELECT id, class_id, user_id, title, content, last_reply_at, created_at FROM discussions WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (m *ClassModel) DeleteDiscussion(id int64) error {
	ctx := context.Background()
	return m.conn.TransactCtx(ctx, func(ctx context.Context, s sqlx.Session) error {
		if _, err := s.ExecCtx(ctx, "DELETE FROM discussion_replies WHERE discussion_id=?", id); err != nil {
			return err
		}
		if _, err := s.ExecCtx(ctx, "DELETE FROM discussions WHERE id=?", id); err != nil {
			return err
		}
		return nil
	})
}

// ---- discussion_replies 回复 ----

type ReplyRow struct {
	Id           int64     `db:"id" json:"id"`
	DiscussionId int64     `db:"discussion_id"`
	UserId       int64     `db:"user_id" json:"userId"`
	Username     string    `db:"username" json:"username"`
	Content      string    `db:"content" json:"content"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
}

func (m *ClassModel) InsertReply(r *ReplyRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO discussion_replies (discussion_id, user_id, content) VALUES (?,?,?)",
		r.DiscussionId, r.UserId, r.Content)
	if err != nil {
		return 0, err
	}
	// 更新帖子最后回复时间
	_, _ = m.conn.ExecCtx(context.Background(),
		"UPDATE discussions SET last_reply_at=NOW() WHERE id=?", r.DiscussionId)
	return res.LastInsertId()
}

func (m *ClassModel) ListReplies(discussionId int64) ([]*ReplyRow, error) {
	var rows []*ReplyRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT r.id, r.discussion_id, r.user_id, u.username, r.content, r.created_at
		 FROM discussion_replies r JOIN users u ON r.user_id=u.id
		 WHERE r.discussion_id=? ORDER BY r.created_at`, discussionId)
	return rows, err
}

// ---- 成绩与学情 ----
//
// 说明：成绩册的聚合查询已迁到 model/grade.go（ListGradebookRaw），
// 这里只保留"权重规则"这类班级配置。旧的 ListGrades 子查询实现已删除：
// 它把视频项写死成满分、签到不封顶、还丢掉了考试权重，算出来的总分没有意义。

// GetGradeRule 取班级成绩权重
func (m *ClassModel) GetGradeRule(classId int64) (videoW, assignW, signinW, examW int, err error) {
	type rule struct {
		Video  int `db:"video_weight"`
		Assign int `db:"assignment_weight"`
		Signin int `db:"signin_weight"`
		Exam   int `db:"exam_weight"`
	}
	var r rule
	err = m.conn.QueryRowCtx(context.Background(), &r,
		"SELECT video_weight, assignment_weight, signin_weight, exam_weight FROM class_grade_rules WHERE class_id=?", classId)
	if err != nil {
		// 没配过就用默认
		return 30, 50, 10, 10, nil
	}
	return r.Video, r.Assign, r.Signin, r.Exam, nil
}

// ---- 作业客观题 ----

type QuestionRow struct {
	Id           int64  `db:"id" json:"id"`
	AssignmentId int64  `db:"assignment_id" json:"assignmentId"`
	Type         string `db:"type" json:"type"` // single/multiple/judge/fill/essay/fileupload
	Stem         string `db:"stem" json:"stem"`
	Options      string `db:"options" json:"options"`
	Answer       string `db:"answer" json:"answer"`
	Score        int    `db:"score" json:"score"`
	OrderNum     int    `db:"order_num" json:"orderNum"`
	Required     int    `db:"required" json:"required"`
}

func (m *ClassModel) InsertQuestion(q *QuestionRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO assignment_questions (assignment_id, type, stem, options, answer, score, order_num, required) VALUES (?,?,?,?,?,?,?,?)",
		q.AssignmentId, q.Type, q.Stem, q.Options, q.Answer, q.Score, q.OrderNum, q.Required)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (m *ClassModel) ListQuestions(assignmentId int64) ([]*QuestionRow, error) {
	var rows []*QuestionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, assignment_id, type, stem, options, answer, score, order_num, required FROM assignment_questions WHERE assignment_id=? ORDER BY order_num ASC, id ASC",
		assignmentId)
	return rows, err
}

// UpdateAssignmentQuestion 编辑作业单题。
func (m *ClassModel) UpdateAssignmentQuestion(q *QuestionRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE assignment_questions SET type=?, stem=?, options=?, answer=?, score=?, required=? WHERE id=? AND assignment_id=?`,
		q.Type, q.Stem, q.Options, q.Answer, q.Score, q.Required, q.Id, q.AssignmentId)
	return err
}

// DeleteAssignmentQuestion 删作业题。
func (m *ClassModel) DeleteAssignmentQuestion(qid int64) error {
	_, err := m.conn.ExecCtx(context.Background(), "DELETE FROM assignment_questions WHERE id=?", qid)
	return err
}

// GetAssignmentQuestion 取作业单题。
func (m *ClassModel) GetAssignmentQuestion(qid int64) (*QuestionRow, error) {
	var q QuestionRow
	err := m.conn.QueryRowCtx(context.Background(), &q,
		"SELECT id, assignment_id, type, stem, options, answer, score, order_num, required FROM assignment_questions WHERE id=?", qid)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// SortAssignmentQuestions 批量改排序。
func (m *ClassModel) SortAssignmentQuestions(assignmentId int64, order []int64) error {
	return m.conn.TransactCtx(context.Background(), func(ctx context.Context, tx sqlx.Session) error {
		for i, qid := range order {
			if _, err := tx.ExecCtx(ctx,
				"UPDATE assignment_questions SET order_num=? WHERE id=? AND assignment_id=?", i+1, qid, assignmentId); err != nil {
				return err
			}
		}
		return nil
	})
}

// MaxAssignmentQuestionOrder 查某作业下当前最大 order_num。
func (m *ClassModel) MaxAssignmentQuestionOrder(assignmentId int64) (int, error) {
	var n int
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COALESCE(MAX(order_num),0) FROM assignment_questions WHERE assignment_id=?", assignmentId)
	return n, err
}

// UpdateAssignment 更新作业基本信息。
func (m *ClassModel) UpdateAssignment(a *AssignmentRow) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE assignments SET title=?, description=?, deadline=?, total_score=?, allow_late=?, late_deadline=? WHERE id=?`,
		a.Title, a.Description, a.Deadline, a.TotalScore, a.AllowLate, a.LateDeadline, a.Id)
	return err
}

// ListAssignmentsPaged 作业列表分页。
func (m *ClassModel) ListAssignmentsPaged(classId int64, page, size int) ([]*AssignmentRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	var total int64
	if err := m.conn.QueryRowCtx(context.Background(), &total,
		"SELECT COUNT(*) FROM assignments WHERE class_id=?", classId); err != nil {
		return nil, 0, err
	}
	var rows []*AssignmentRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, course_id, title, description, deadline, total_score, allow_late, late_deadline, publisher_id, created_at FROM assignments WHERE class_id=? ORDER BY created_at DESC LIMIT ? OFFSET ?",
		classId, size, offset)
	return rows, total, err
}

// CountQuestions 统计某作业客观题总分（用于自动判分参考）
func (m *ClassModel) SumQuestionScore(assignmentId int64) (int, error) {
	var total int
	err := m.conn.QueryRowCtx(context.Background(), &total,
		"SELECT COALESCE(SUM(score),0) FROM assignment_questions WHERE assignment_id=?", assignmentId)
	return total, err
}

// UpdateSubmissionAutoScore 写回客观题自动得分
func (m *ClassModel) UpdateSubmissionAutoScore(submissionId int64, answers string, autoScore int) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE assignment_submissions SET answers=?, auto_score=? WHERE id=?", answers, autoScore, submissionId)
	return err
}

// ---- 公告已读未读 ----

// MarkAnnouncementRead 记录当前用户已读某公告（唯一键冲突忽略，幂等）
func (m *ClassModel) MarkAnnouncementRead(announcementId, userId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO announcement_reads (announcement_id, user_id) VALUES (?,?)",
		announcementId, userId)
	return err
}

// ListReadUserIds 返回某公告所有已读用户 id（老师端统计已读名单）
func (m *ClassModel) ListReadUserIds(announcementId int64) ([]int64, error) {
	var ids []int64
	err := m.conn.QueryRowsCtx(context.Background(), &ids,
		"SELECT user_id FROM announcement_reads WHERE announcement_id=?", announcementId)
	return ids, err
}

// AnnouncementReadStat 一条公告的已读聚合结果。
type AnnouncementReadStat struct {
	AnnouncementId int64
	ReadCount      int
	ReadByMe       bool
}

// AnnouncementReadStats 批量取"某页公告"的已读人数 + 我是否已读。
//
// 为什么提供批量接口：列表页原来对每条公告各查一次 announcement_reads（N+1），
// 一个班 20 条公告就是 20 次往返，还额外把每条的完整已读名单拉回 Go 再遍历。
// 这里用一条 GROUP BY 把整页的计数与"我是否读过"一次算完：
//   - 行数从 O(N×已读人数) 降到 O(N)；
//   - 往返从 N 次降到 1 次；
//   - 走 uk_ann_user(announcement_id, user_id) 的 IN 范围扫描。
//
// 返回 map，调用方按需取值；没有已读记录的公告不会出现在 map 里（即 0 / false）。
func (m *ClassModel) AnnouncementReadStats(userId int64, announcementIds []int64) (map[int64]AnnouncementReadStat, error) {
	out := make(map[int64]AnnouncementReadStat, len(announcementIds))
	if len(announcementIds) == 0 {
		return out, nil
	}
	holders := strings.TrimSuffix(strings.Repeat("?,", len(announcementIds)), ",")
	args := make([]interface{}, 0, len(announcementIds)+1)
	args = append(args, userId)
	for _, id := range announcementIds {
		args = append(args, id)
	}
	var rows []struct {
		AnnouncementId int64 `db:"announcement_id"`
		ReadCount      int   `db:"read_count"`
		ReadByMe       int   `db:"read_by_me"`
	}
	// 注意：SELECT 里的占位符在 SQL 文本中先于 WHERE，参数顺序必须是 userId 在前。
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT announcement_id,
		        COUNT(*) AS read_count,
		        MAX(CASE WHEN user_id = ? THEN 1 ELSE 0 END) AS read_by_me
		   FROM announcement_reads
		  WHERE announcement_id IN (`+holders+`)
		  GROUP BY announcement_id`, args...)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out[r.AnnouncementId] = AnnouncementReadStat{
			AnnouncementId: r.AnnouncementId,
			ReadCount:      r.ReadCount,
			ReadByMe:       r.ReadByMe == 1,
		}
	}
	return out, nil
}

// CountUnreadByClass 返回当前用户在某班未读公告数
func (m *ClassModel) CountUnreadByClass(classId, userId int64) (int64, error) {
	var n int64
	err := m.conn.QueryRowCtx(context.Background(), &n,
		`SELECT COUNT(*) FROM announcements a
		 WHERE a.class_id=? AND NOT EXISTS (
		   SELECT 1 FROM announcement_reads r WHERE r.announcement_id=a.id AND r.user_id=?
		 )`, classId, userId)
	return n, err
}

// ---- 班级签到 ----

type SigninRow struct {
	Id        int64     `db:"id" json:"id"`
	ClassId   int64     `db:"class_id" json:"classId"`
	Title     string    `db:"title" json:"title"`
	StartAt   time.Time `db:"start_at" json:"startAt"`
	TeacherId int64     `db:"teacher_id" json:"teacherId"`
	Code      string    `db:"code" json:"code"`
	ExpiresAt time.Time `db:"expires_at" json:"expiresAt"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

type SigninRecordRow struct {
	Id       int64     `db:"id" json:"id"`
	SigninId int64     `db:"signin_id" json:"signinId"`
	UserId   int64     `db:"user_id" json:"userId"`
	Username string    `db:"username" json:"username"`
	SignedAt time.Time `db:"signed_at" json:"signedAt"`
}

// CreateSignin 发起一次签到
func (m *ClassModel) CreateSignin(s *SigninRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO class_signin (class_id, title, start_at, teacher_id, code, expires_at) VALUES (?,?,?,?,?,?)",
		s.ClassId, s.Title, s.StartAt, s.TeacherId, s.Code, s.ExpiresAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetActiveSignin 取某班当前进行中的签到（start_at <= now() <= expires_at）
func (m *ClassModel) GetActiveSignin(classId int64) (*SigninRow, error) {
	var s SigninRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, class_id, title, start_at, teacher_id, code, expires_at, created_at FROM class_signin WHERE class_id=? AND start_at<=NOW() AND expires_at>NOW() ORDER BY id DESC LIMIT 1",
		classId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListSigninsByClass 列出班级全部签到事件（老师端看历史 + 进行中）
func (m *ClassModel) ListSigninsByClass(classId int64) ([]*SigninRow, error) {
	var rows []*SigninRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, class_id, title, start_at, teacher_id, code, expires_at, created_at FROM class_signin WHERE class_id=? ORDER BY id DESC",
		classId)
	return rows, err
}

// GetSignin 按 id 取单条签到事件
func (m *ClassModel) GetSignin(id int64) (*SigninRow, error) {
	var s SigninRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, class_id, title, start_at, teacher_id, code, expires_at, created_at FROM class_signin WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetSigninByCode 学生凭签到码查签到
func (m *ClassModel) GetSigninByCode(code string) (*SigninRow, error) {
	var s SigninRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, class_id, title, start_at, teacher_id, code, expires_at, created_at FROM class_signin WHERE code=? ORDER BY id DESC LIMIT 1",
		code)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// EndSignin 老师手动结束签到：把 expires_at 改成 now()，
// 这样 GetActiveSignin 的 expires_at>NOW() 条件自然不命中，签到立即失效。
func (m *ClassModel) EndSignin(id int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE class_signin SET expires_at=NOW() WHERE id=?", id)
	return err
}

// AddSigninRecord 学生签到。
// 利用 (signin_id, user_id) 唯一键 + INSERT IGNORE：
// 返回 inserted=true 表示本次是第一次签到；
// 返回 inserted=false 表示该学生已签过到（重复签到），调用方应向前端报错而不是假装成功。
func (m *ClassModel) AddSigninRecord(signinId, userId int64) (inserted bool, err error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT IGNORE INTO signin_records (signin_id, user_id) VALUES (?,?)", signinId, userId)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// ListSigninRecords 老师查看签到名单
func (m *ClassModel) ListSigninRecords(signinId int64) ([]*SigninRecordRow, error) {
	var rows []*SigninRecordRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT r.id, r.signin_id, r.user_id, u.username, r.signed_at
		 FROM signin_records r JOIN users u ON r.user_id=u.id
		 WHERE r.signin_id=? ORDER BY r.signed_at`, signinId)
	return rows, err
}

// ---- 站内通知 ----

type NotificationRow struct {
	Id        int64     `db:"id" json:"id"`
	UserId    int64     `db:"user_id" json:"userId"`
	Type      string    `db:"type" json:"type"`
	Title     string    `db:"title" json:"title"`
	Content   string    `db:"content" json:"content"`
	LinkUrl   string    `db:"link_url" json:"linkUrl"`
	IsRead    bool      `db:"is_read" json:"isRead"`
	CreatedAt time.Time `db:"created_at" json:"createdAt"`
}

// InsertNotification 给单个用户写一条通知
func (m *ClassModel) InsertNotification(n *NotificationRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO notifications (user_id, type, title, content, link_url) VALUES (?,?,?,?,?)",
		n.UserId, n.Type, n.Title, n.Content, n.LinkUrl)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// BroadcastNotification 给班级内除 senderId 外的所有成员批量写一条通知。
// 用 INSERT ... SELECT 一条 SQL 完成，避免逐条 INSERT。
func (m *ClassModel) BroadcastNotification(classId int64, senderId int64, nType, title, content, linkUrl string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO notifications (user_id, type, title, content, link_url)
		 SELECT user_id, ?, ?, ?, ? FROM class_members
		 WHERE class_id=? AND user_id<>?`,
		nType, title, content, linkUrl, classId, senderId)
	return err
}

// ListNotifications 列出某用户的通知（按时间倒序）
func (m *ClassModel) ListNotifications(userId int64, limit int) ([]*NotificationRow, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []*NotificationRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, user_id, type, title, content, link_url, is_read, created_at FROM notifications WHERE user_id=? ORDER BY id DESC LIMIT ?",
		userId, limit)
	return rows, err
}

// CountUnreadNotifications 某用户未读通知数
func (m *ClassModel) CountUnreadNotifications(userId int64) (int64, error) {
	var n int64
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM notifications WHERE user_id=? AND is_read=0", userId)
	return n, err
}

// MarkNotificationRead 标记通知已读（幂等）
func (m *ClassModel) MarkNotificationRead(id, userId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE notifications SET is_read=1 WHERE id=? AND user_id=?", id, userId)
	return err
}

// MarkAllNotificationsRead 全部标已读
func (m *ClassModel) MarkAllNotificationsRead(userId int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE notifications SET is_read=1 WHERE user_id=? AND is_read=0", userId)
	return err
}

// ---- 邀请码：查未过期邀请码（用于问题9：只能生成一次） ----

// GetActiveInvitation 返回该班当前未过期、未用满的邀请码；没有则返回 (nil, nil)
func (m *ClassModel) GetActiveInvitation(classId int64) (*InvitationRow, error) {
	var inv InvitationRow
	err := m.conn.QueryRowCtx(context.Background(), &inv,
		"SELECT id, class_id, code, expires_at, max_uses, used_count FROM class_invitations WHERE class_id=? ORDER BY id DESC LIMIT 1",
		classId)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &inv, nil
}
