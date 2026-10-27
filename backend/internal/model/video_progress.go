package model

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ===== 学习进度（对标 Open edX progress 模型）=====
//
// 关键区别（相比改造前把 position 写进 Redis key）：
//   - 按 (user_id, video_id) 落库，不再全班共用一个进度；
//   - 记录 watched_seconds（累计有效观看），完成度看它，而不是看"播到第几秒"，
//     否则拖到 90% 会被判定成"看完了"；
//   - duration_seconds 由播放器上报：服务端没有可靠媒资时长来源，
//     视频元数据只有 size/url（ffprobe 只在 faststart 流程里用过，未落库）。

// VideoProgressRow 一个用户在某视频上的学习进度。
type VideoProgressRow struct {
	Id              int64     `db:"id" json:"id"`
	UserId          int64     `db:"user_id" json:"userId"`
	CourseId        int64     `db:"course_id" json:"courseId"`
	VideoId         int64     `db:"video_id" json:"videoId"`
	PositionSeconds int       `db:"position_seconds" json:"positionSeconds"`
	WatchedSeconds  int       `db:"watched_seconds" json:"watchedSeconds"`
	DurationSeconds int       `db:"duration_seconds" json:"durationSeconds"`
	Completed       bool      `db:"completed" json:"completed"`
	UpdatedAt       time.Time `db:"updated_at" json:"updatedAt"`
}

// completionRatio 看完的判定阈值：累计观看达到时长的 90%。
const completionRatio = 0.9

// GetVideoProgress 读某用户在某视频上的进度，没有记录返回 (nil, nil)。
func (m *CourseModel) GetVideoProgress(userId, videoId int64) (*VideoProgressRow, error) {
	var p VideoProgressRow
	err := m.conn.QueryRowCtx(context.Background(), &p,
		`SELECT id, user_id, course_id, video_id, position_seconds, watched_seconds, duration_seconds, completed, updated_at
		   FROM video_progress WHERE user_id=? AND video_id=?`, userId, videoId)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SaveVideoProgress 上报一次播放进度。
//
// watchedDelta 是"上次上报以来真实播放过的秒数"（前端按 playbackRate 累加，
// 拖进度条不计入）。累加、封顶、完成标记都在 NextVideoProgress 这个纯函数里算，
// 落库用 GREATEST，避免多标签页/乱序请求把进度写小。
//
// 性能：改造前是"读一次 → 写一次 → 再读一次"（3 次往返），而这是全站最高频的
// 写路径（播放器每 15 秒上报一次，100 人在看就是 ~6.7 QPS 的纯写）。
// 现在写完直接返回本次计算出的状态，去掉最后一次 SELECT，降到 2 次往返。
func (m *CourseModel) SaveVideoProgress(userId, courseId, videoId int64, position, watchedDelta, duration int) (*VideoProgressRow, error) {
	prev, err := m.GetVideoProgress(userId, videoId)
	if err != nil {
		return nil, err
	}
	next := NextVideoProgress(prev, position, watchedDelta, duration)
	next.UserId = userId
	next.VideoId = videoId
	if next.CourseId == 0 {
		// 首次写入用请求里的课程；重复上报保留库里已有的 course_id（唯一键是 user+video）
		next.CourseId = courseId
	}

	_, err = m.conn.ExecCtx(context.Background(),
		`INSERT INTO video_progress (user_id, course_id, video_id, position_seconds, watched_seconds, duration_seconds, completed)
		 VALUES (?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
		   position_seconds = VALUES(position_seconds),
		   watched_seconds  = GREATEST(watched_seconds, VALUES(watched_seconds)),
		   duration_seconds = GREATEST(duration_seconds, VALUES(duration_seconds)),
		   completed        = GREATEST(completed, VALUES(completed))`,
		next.UserId, next.CourseId, next.VideoId,
		next.PositionSeconds, next.WatchedSeconds, next.DurationSeconds, boolToTinyint(next.Completed))
	if err != nil {
		return nil, err
	}
	return next, nil
}

// NextVideoProgress 计算一次上报后的进度（纯函数，便于穷举单测）。
//
// 规则：
//   - 观看时长累加，且不超过已知的总时长（时长未知时不封顶）；
//   - 总时长取历史值与本次上报的较大者（播放器偶尔会上报 0）；
//   - 完成标记只增不减：后续乱序/重复上报不能把"已看完"改回未看完。
func NextVideoProgress(prev *VideoProgressRow, position, watchedDelta, duration int) *VideoProgressRow {
	if position < 0 {
		position = 0
	}
	if watchedDelta < 0 {
		watchedDelta = 0
	}
	if duration < 0 {
		duration = 0
	}
	row := &VideoProgressRow{
		PositionSeconds: position,
		WatchedSeconds:  watchedDelta,
		DurationSeconds: duration,
	}
	if prev != nil {
		row.Id = prev.Id
		row.CourseId = prev.CourseId
		row.WatchedSeconds = prev.WatchedSeconds + watchedDelta
		if prev.DurationSeconds > row.DurationSeconds {
			row.DurationSeconds = prev.DurationSeconds
		}
		row.Completed = prev.Completed
	}
	if row.DurationSeconds > 0 && row.WatchedSeconds > row.DurationSeconds {
		row.WatchedSeconds = row.DurationSeconds
	}
	if row.DurationSeconds > 0 && float64(row.WatchedSeconds) >= completionRatio*float64(row.DurationSeconds) {
		row.Completed = true
	}
	return row
}

func boolToTinyint(v bool) int {
	if v {
		return 1
	}
	return 0
}

// ListVideoProgressByCourse 某用户在某课程下的全部进度（课程详情页显示"已看 x/y"用）。
func (m *CourseModel) ListVideoProgressByCourse(userId, courseId int64) ([]*VideoProgressRow, error) {
	var rows []*VideoProgressRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT id, user_id, course_id, video_id, position_seconds, watched_seconds, duration_seconds, completed, updated_at
		   FROM video_progress WHERE user_id=? AND course_id=?`, userId, courseId)
	return rows, err
}

// CountCourseVideos 某课程的视频总数。
func (m *CourseModel) CountCourseVideos(courseId int64) (int64, error) {
	var n int64
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM course_videos WHERE course_id=?", courseId)
	return n, err
}

// CountUserCompletedInCourse 某用户在某课程下已看完的视频数。
func (m *CourseModel) CountUserCompletedInCourse(userId, courseId int64) (int64, error) {
	var n int64
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM video_progress WHERE user_id=? AND course_id=? AND completed=1", userId, courseId)
	return n, err
}
