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
	VideoCount    int64  `db:"video_count"`
}

// VideoRow 对应 course_videos 表一行。
type VideoRow struct {
	Id          int64  `db:"id"`
	CourseId    int64  `db:"course_id"`
	Name        string `db:"name"`
	Url         string `db:"url"`
	ObjectName  string `db:"object_name"`
	Size        int64  `db:"size"`
	Faststarted bool   `db:"faststarted"`
	// 后处理（faststart）状态。这一列是"上传即排队"的载体：
	// worker 靠条件更新认领 pending 行，所以进程重启后没跑完的任务会自动续跑。
	// 表里还有 processed_at（识别卡死 processing 用），只在 SQL 里比较，不映射进来。
	ProcessStatus   string    `db:"process_status"`
	ProcessError    string    `db:"process_error"`
	ProcessAttempts int       `db:"process_attempts"`
	UploadedAt      time.Time `db:"uploaded_at"`
}

// 视频后处理状态（与 V24 迁移里 process_status 的取值一一对应）。
const (
	VideoProcessPending    = "pending"    // 待处理，worker 会捞
	VideoProcessProcessing = "processing" // 已被某个 worker 认领
	VideoProcessDone       = "done"       // 已生成 faststart 版本
	VideoProcessFailed     = "failed"     // 超过重试上限，等人工/前端重试
)

// 重新排队（前端"重试"）的三种拒绝原因，调用方按 sentinel 翻译成 404/400。
var (
	ErrVideoNotFound    = errors.New("视频不存在")
	ErrVideoAlreadyDone = errors.New("该视频已处理完成，无需重试")
	ErrVideoNotFailed   = errors.New("该视频正在排队或处理中，无需重试")
)

// videoColumns ListVideos / FindVideo / ListPendingVideos 共用的列清单。
// 必须与 VideoRow 的 db 字段一一对应 —— go-zero 严格扫描下，
// 少一列就整条查询报错（这正是 2026-11-07 课程查询全线失败的原因）。
const videoColumns = "id, course_id, name, url, object_name, size, faststarted, " +
	"process_status, process_error, process_attempts, uploaded_at"

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

// FindCourseById 按 id 查课程，不存在返回 model.ErrNotFound。
// 用 Partial：CourseRow 有 video_count 字段但本查询不取它，严格模式会因列数少于字段数直接报错。
func (m *CourseModel) FindCourseById(id int64) (*CourseRow, error) {
	var c CourseRow
	err := m.conn.QueryRowPartialCtx(context.Background(), &c,
		"SELECT id, name, intro, teacher, created_by, cover_url, cover_thumb_url FROM courses WHERE id = ?", id)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCourses 全部课程（按 id 升序）。同样用 Partial，理由见 FindCourseById。
func (m *CourseModel) ListCourses() ([]*CourseRow, error) {
	var rows []*CourseRow
	err := m.conn.QueryRowsPartialCtx(context.Background(), &rows,
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
		`SELECT c.id, c.name, c.intro, c.teacher, c.created_by, c.cover_url, c.cover_thumb_url,
		        (SELECT COUNT(*) FROM course_videos v WHERE v.course_id = c.id) AS video_count
		 FROM courses c ORDER BY c.id DESC LIMIT ? OFFSET ?`,
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
// process_status 由调用方给（一般是 pending），空值兜底成 pending，避免出现"没人排队"的行。
func (m *CourseModel) InsertVideos(courseId int64, videos []*VideoRow) error {
	ctx := context.Background()
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		for _, v := range videos {
			status := v.ProcessStatus
			if status == "" {
				status = VideoProcessPending
			}
			res, err := session.ExecCtx(ctx,
				"INSERT INTO course_videos (course_id, name, url, object_name, size, faststarted, process_status) VALUES (?, ?, ?, ?, ?, ?, ?)",
				courseId, v.Name, v.Url, v.ObjectName, v.Size, v.Faststarted, status)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			v.Id = id
			v.ProcessStatus = status
		}
		return nil
	})
}

// ListVideos 查某课程全部视频（按 id 升序，即上传顺序）。
func (m *CourseModel) ListVideos(courseId int64) ([]*VideoRow, error) {
	var rows []*VideoRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT "+videoColumns+" FROM course_videos WHERE course_id = ? ORDER BY id ASC", courseId)
	return rows, err
}

// FindVideo 按课程 + 视频 id 查单个视频，不存在返回 sql.ErrNoRows。
func (m *CourseModel) FindVideo(courseId, videoId int64) (*VideoRow, error) {
	var v VideoRow
	err := m.conn.QueryRowCtx(context.Background(), &v,
		"SELECT "+videoColumns+" FROM course_videos WHERE course_id = ? AND id = ?",
		courseId, videoId)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ===== course_videos 后处理队列（faststart worker 用）=====

// ListPendingVideos 捞最多 limit 个待处理视频（按 id 升序，先上传的先处理）。
// 只读不改状态，认领靠 ClaimPendingVideo。
func (m *CourseModel) ListPendingVideos(limit int) ([]*VideoRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 8
	}
	var rows []*VideoRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		"SELECT "+videoColumns+" FROM course_videos WHERE process_status = ? ORDER BY id ASC LIMIT ?",
		VideoProcessPending, limit)
	return rows, err
}

// ClaimPendingVideo 原子认领一个 pending 视频：认领成功才返回 true。
// 条件是 process_status='pending'，所以同一行被多个 worker / 多实例抢到只会有一个 true。
// 顺手把 attempts +1、processed_at 刷新（后者用来识别"认领后进程死了"的行）。
func (m *CourseModel) ClaimPendingVideo(id int64) (bool, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		`UPDATE course_videos
		    SET process_status = ?, process_attempts = process_attempts + 1, processed_at = NOW()
		  WHERE id = ? AND process_status = ?`,
		VideoProcessProcessing, id, VideoProcessPending)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// FinishVideoProcess 写回处理结果。
//   - done：换成 faststart 版本的 URL、置 faststarted=1、清空错误；
//   - pending（本轮失败但还能重试）：保留原 URL，记下错误原因等下一轮；
//   - failed：不再自动重试，前端给"重试"按钮。
func (m *CourseModel) FinishVideoProcess(id int64, status, newUrl, procErr string) error {
	if status == VideoProcessDone {
		_, err := m.conn.ExecCtx(context.Background(),
			`UPDATE course_videos
			    SET url = ?, faststarted = 1, process_status = ?, process_error = '', processed_at = NOW()
			  WHERE id = ?`,
			newUrl, VideoProcessDone, id)
		return err
	}
	if len(procErr) > 500 {
		procErr = procErr[:500]
	}
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE course_videos
		    SET process_status = ?, process_error = ?, processed_at = NOW()
		  WHERE id = ?`,
		status, procErr, id)
	return err
}

// ResetStaleProcessingVideos 把"卡在 processing 超过 after"的行退回 pending。
// 场景：worker 认领后进程被部署/崩溃打断，没人再写回状态。
// after 必须明显大于单次 ffmpeg 超时（10 分钟）+ 整片下载时间，否则会抢正在跑的任务。
func (m *CourseModel) ResetStaleProcessingVideos(after time.Duration) (int64, error) {
	minutes := int(after.Minutes())
	if minutes < 1 {
		minutes = 1
	}
	res, err := m.conn.ExecCtx(context.Background(),
		`UPDATE course_videos
		    SET process_status = ?, process_error = '处理中断（服务重启或异常），已重新排队', processed_at = NOW()
		  WHERE process_status = ?
		    AND (processed_at IS NULL OR processed_at < DATE_SUB(NOW(), INTERVAL ? MINUTE))`,
		VideoProcessPending, VideoProcessProcessing, minutes)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// RequeueVideoForProcess 手工把视频重新排队（前端"重试"按钮）。
// 只接受 failed 行 —— done 再跑一遍是白烧 CPU，pending/processing 本来就在队列里。
// 更新影响 0 行时再查一次，区分"视频不存在"和"状态不对"，两者前端文案不同。
func (m *CourseModel) RequeueVideoForProcess(courseId, videoId int64) error {
	res, err := m.conn.ExecCtx(context.Background(),
		`UPDATE course_videos
		    SET process_status = ?, process_attempts = 0, process_error = '', processed_at = NOW()
		  WHERE course_id = ? AND id = ? AND process_status = ?`,
		VideoProcessPending, courseId, videoId, VideoProcessFailed)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return nil
	}
	row, err := m.FindVideo(courseId, videoId)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrVideoNotFound
		}
		return err
	}
	if row.ProcessStatus == VideoProcessDone {
		return ErrVideoAlreadyDone
	}
	return ErrVideoNotFailed
}

// MarkVideoFaststart 标记视频已 faststart，并替换播放 URL。
// 保留给旧调用方；新的 worker 走 FinishVideoProcess（同时写状态）。
func (m *CourseModel) MarkVideoFaststart(courseId, videoId int64, newUrl string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE course_videos SET url = ?, faststarted = 1, process_status = ?, process_error = '', processed_at = NOW() WHERE course_id = ? AND id = ?",
		newUrl, VideoProcessDone, courseId, videoId)
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
