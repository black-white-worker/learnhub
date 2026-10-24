package model

import (
	"context"
	"database/sql"
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
	Id            int64          `db:"id"`
	Name          string         `db:"name"`
	Grade         string         `db:"grade"`
	Major         string         `db:"major"`
	HeadTeacherId int64          `db:"head_teacher_id"`
	CreatedBy     string         `db:"created_by"`
	CreatedAt     time.Time      `db:"created_at"`
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
	Id       int64  `db:"id"`
	ClassId  int64  `db:"class_id"`
	UserId   int64  `db:"user_id"`
	Role     string `db:"role"`
	Username string `db:"username"`
	Nickname string `db:"nickname"`
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

func (m *ClassModel) ListClassCourseIds(classId int64) ([]int64, error) {
	var ids []int64
	err := m.conn.QueryRowsCtx(context.Background(), &ids,
		"SELECT course_id FROM class_courses WHERE class_id=?", classId)
	return ids, err
}

// ---- class_invitations 表 ----

type InvitationRow struct {
	Id        int64        `db:"id"`
	ClassId   int64        `db:"class_id"`
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
	Id         int64     `db:"id"`
	ClassId    int64     `db:"class_id"`
	Title      string    `db:"title"`
	Content    string    `db:"content"`
	PublisherId int64    `db:"publisher_id"`
	IsTop      bool      `db:"is_top"`
	CreatedAt  time.Time `db:"created_at"`
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

func (m *ClassModel) DeleteAnnouncement(id int64) error {
	_, err := m.conn.ExecCtx(context.Background(), "DELETE FROM announcements WHERE id=?", id)
	return err
}

// ---- assignments 作业 ----

type AssignmentRow struct {
	Id         int64      `db:"id"`
	ClassId    int64      `db:"class_id"`
	CourseId   int64      `db:"course_id"`
	Title      string     `db:"title"`
	Description string    `db:"description"`
	Deadline   sql.NullTime `db:"deadline"`
	TotalScore int        `db:"total_score"`
	PublisherId int64     `db:"publisher_id"`
	CreatedAt  time.Time  `db:"created_at"`
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
		"SELECT id, class_id, course_id, title, description, deadline, total_score, publisher_id, created_at FROM assignments WHERE class_id=? ORDER BY created_at DESC", classId)
	return rows, err
}

func (m *ClassModel) GetAssignment(id int64) (*AssignmentRow, error) {
	var a AssignmentRow
	err := m.conn.QueryRowCtx(context.Background(), &a,
		"SELECT id, class_id, course_id, title, description, deadline, total_score, publisher_id, created_at FROM assignments WHERE id=?", id)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// ---- assignment_submissions 作业提交 ----

type SubmissionRow struct {
	Id          int64          `db:"id"`
	AssignmentId int64         `db:"assignment_id"`
	UserId      int64          `db:"user_id"`
	Content     string         `db:"content"`
	FileUrl     string         `db:"file_url"`
	Score       sql.NullInt64  `db:"score"`
	Comment     string         `db:"comment"`
	SubmittedAt time.Time      `db:"submitted_at"`
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

func (m *ClassModel) ListSubmissions(assignmentId int64) ([]*SubmissionRow, error) {
	var rows []*SubmissionRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, assignment_id, user_id, content, file_url, score, comment, submitted_at FROM assignment_submissions WHERE assignment_id=? ORDER BY submitted_at DESC", assignmentId)
	return rows, err
}

func (m *ClassModel) GetMySubmission(assignmentId, userId int64) (*SubmissionRow, error) {
	var s SubmissionRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, assignment_id, user_id, content, file_url, score, comment, submitted_at FROM assignment_submissions WHERE assignment_id=? AND user_id=?",
		assignmentId, userId)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ---- discussions 讨论 ----

type DiscussionRow struct {
	Id           int64     `db:"id"`
	ClassId      int64     `db:"class_id"`
	UserId       int64     `db:"user_id"`
	Title        string    `db:"title"`
	Content      string    `db:"content"`
	LastReplyAt  time.Time `db:"last_reply_at"`
	CreatedAt    time.Time `db:"created_at"`
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
	Id           int64     `db:"id"`
	DiscussionId int64     `db:"discussion_id"`
	UserId       int64     `db:"user_id"`
	Content      string    `db:"content"`
	CreatedAt    time.Time `db:"created_at"`
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
		"SELECT id, discussion_id, user_id, content, created_at FROM discussion_replies WHERE discussion_id=? ORDER BY created_at", discussionId)
	return rows, err
}
