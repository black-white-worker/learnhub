package model

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// CourseRow 对应 courses 表一行。
type CourseRow struct {
	Id            int64  `db:"id"`
	Name          string `db:"name"`
	Intro         string `db:"intro"`
	Teacher       string `db:"teacher"`
	CreatedBy     string `db:"created_by"`
	CoverUrl      string `db:"cover_url"`
	CoverThumbUrl string `db:"cover_thumb_url"`
}

// VideoRow 对应 course_videos 表一行。
type VideoRow struct {
	Id          int64     `db:"id"`
	CourseId    int64     `db:"course_id"`
	Name        string    `db:"name"`
	Url         string    `db:"url"`
	ObjectName  string    `db:"object_name"`
	Size        int64     `db:"size"`
	Faststarted bool      `db:"faststarted"`
	UploadedAt  time.Time `db:"uploaded_at"`
}

// ResourceRow 对应 course_resources 表一行。
type ResourceRow struct {
	Id         int64     `db:"id"`
	CourseId   int64     `db:"course_id"`
	Name       string    `db:"name"`
	Url        string    `db:"url"`
	ObjectName string    `db:"object_name"`
	Type       string    `db:"type"`
	Size       int64     `db:"size"`
	UploadedAt time.Time `db:"uploaded_at"`
}

// CourseModel 课程模块数据访问层（MySQL）。
type CourseModel struct {
	conn sqlx.SqlConn
}

// NewCourseModel 创建课程数据访问层。
func NewCourseModel(conn sqlx.SqlConn) *CourseModel {
	return &CourseModel{conn: conn}
}

// ===== courses 表 =====

// InsertCourse 新增课程，返回自增 id。重名（唯一索引）时返回"课程名称已存在"。
func (m *CourseModel) InsertCourse(name, intro, teacher, createdBy string) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO courses (name, intro, teacher, created_by) VALUES (?, ?, ?, ?)", name, intro, teacher, createdBy)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return 0, errors.New("课程名称已存在")
		}
		return 0, err
	}
	return res.LastInsertId()
}

// FindCourseById 按 id 查课程，不存在返回 sql.ErrNoRows。
func (m *CourseModel) FindCourseById(id int64) (*CourseRow, error) {
	var c CourseRow
	err := m.conn.QueryRowCtx(context.Background(), &c,
		"SELECT id, name, intro, teacher, created_by, cover_url, cover_thumb_url FROM courses WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCourses 全部课程（按 id 升序）。
func (m *CourseModel) ListCourses() ([]*CourseRow, error) {
	var rows []*CourseRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, name, intro, teacher, created_by, cover_url, cover_thumb_url FROM courses ORDER BY id ASC")
	return rows, err
}

// ListCoursesPaged 课程分页列表。
func (m *CourseModel) ListCoursesPaged(page, size int) ([]*CourseRow, int64, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}
	offset := (page - 1) * size
	var total int64
	if err := m.conn.QueryRowCtx(context.Background(), &total, "SELECT COUNT(*) FROM courses"); err != nil {
		return nil, 0, err
	}
	var rows []*CourseRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, name, intro, teacher, created_by, cover_url, cover_thumb_url FROM courses ORDER BY id DESC LIMIT ? OFFSET ?",
		size, offset)
	return rows, total, err
}

// UpdateCourse 更新课程基本信息。
func (m *CourseModel) UpdateCourse(id int64, name, intro, teacher string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE courses SET name = ?, intro = ?, teacher = ? WHERE id = ?", name, intro, teacher, id)
	return err
}

// UpdateCourseCover 设置封面（原图 + 缩略图）。
func (m *CourseModel) UpdateCourseCover(id int64, coverUrl, coverThumbUrl string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE courses SET cover_url = ?, cover_thumb_url = ? WHERE id = ?", coverUrl, coverThumbUrl, id)
	return err
}

// DeleteCourse 删除课程，并级联删它的视频和资源元数据。
// MinIO 里的文件由孤儿清理兜底。
func (m *CourseModel) DeleteCourse(id int64) error {
	ctx := context.Background()
	// TransactCtx 在回调返 nil 时自动 commit，返非 nil 时自动 rollback
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if _, e := session.ExecCtx(ctx, "DELETE FROM course_videos WHERE course_id = ?", id); e != nil {
			return e
		}
		if _, e := session.ExecCtx(ctx, "DELETE FROM course_resources WHERE course_id = ?", id); e != nil {
			return e
		}
		res, e := session.ExecCtx(ctx, "DELETE FROM courses WHERE id = ?", id)
		if e != nil {
			return e
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return errors.New("课程不存在")
		}
		return nil
	})
}

// ===== course_videos 表 =====

// InsertVideos 批量插入视频，回填每条的自增 id。
func (m *CourseModel) InsertVideos(courseId int64, videos []*VideoRow) error {
	ctx := context.Background()
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		for _, v := range videos {
			res, err := session.ExecCtx(ctx,
				"INSERT INTO course_videos (course_id, name, url, object_name, size, faststarted) VALUES (?, ?, ?, ?, ?, ?)",
				courseId, v.Name, v.Url, v.ObjectName, v.Size, v.Faststarted)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			v.Id = id
		}
		return nil
	})
}

// ListVideos 查某课程全部视频（按 id 升序，即上传顺序）。
func (m *CourseModel) ListVideos(courseId int64) ([]*VideoRow, error) {
	var rows []*VideoRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, course_id, name, url, object_name, size, faststarted, uploaded_at FROM course_videos WHERE course_id = ? ORDER BY id ASC", courseId)
	return rows, err
}

// FindVideo 按课程 + 视频 id 查单个视频，不存在返回 sql.ErrNoRows。
func (m *CourseModel) FindVideo(courseId, videoId int64) (*VideoRow, error) {
	var v VideoRow
	err := m.conn.QueryRowCtx(context.Background(), &v,
		"SELECT id, course_id, name, url, object_name, size, faststarted, uploaded_at FROM course_videos WHERE course_id = ? AND id = ?",
		courseId, videoId)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// MarkVideoFaststart 标记视频已 faststart，并替换播放 URL。
func (m *CourseModel) MarkVideoFaststart(courseId, videoId int64, newUrl string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE course_videos SET url = ?, faststarted = 1 WHERE course_id = ? AND id = ?",
		newUrl, courseId, videoId)
	return err
}

// ===== course_resources 表 =====

// InsertResource 插入一条资源，回填自增 id。
func (m *CourseModel) InsertResource(courseId int64, r *ResourceRow) error {
	res, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO course_resources (course_id, name, url, object_name, type, size) VALUES (?, ?, ?, ?, ?, ?)",
		courseId, r.Name, r.Url, r.ObjectName, r.Type, r.Size)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	r.Id = id
	return nil
}

// ListResources 查某课程全部资源。
func (m *CourseModel) ListResources(courseId int64) ([]*ResourceRow, error) {
	var rows []*ResourceRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT id, course_id, name, url, object_name, type, size, uploaded_at FROM course_resources WHERE course_id = ? ORDER BY id ASC", courseId)
	return rows, err
}

// DeleteResource 删除一条资源，不存在返回"资源不存在"。
func (m *CourseModel) DeleteResource(courseId, resourceId int64) error {
	res, err := m.conn.ExecCtx(context.Background(),
		"DELETE FROM course_resources WHERE course_id = ? AND id = ?", courseId, resourceId)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("资源不存在")
	}
	return nil
}

// ErrNotFound 供上层判断"不存在"。
var ErrNotFound = sql.ErrNoRows
