package model

// 批阅队列（对标 Canvas SpeedGrader 的"逐份批阅"数据源）。
//
// 老师批 40 份作业时，最怕的是"每批一份就整页刷新、翻回列表再点下一份"。
// 这个接口一次返回整份队列（含学生名 + 是否已批），前端在本地做上一份/下一份，
// 只在保存时发一次请求——把每份作业的往返次数从 3 次降到 1 次。

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// SubmissionQueueRow 批阅队列里的一份提交（带学生名）。
type SubmissionQueueRow struct {
	Id           int64         `db:"id" json:"id"`
	AssignmentId int64         `db:"assignment_id" json:"assignmentId"`
	UserId       int64         `db:"user_id" json:"userId"`
	Username     string        `db:"username" json:"username"`
	Content      string        `db:"content" json:"content"`
	Answers      string        `db:"answers" json:"answers"`
	FileUrl      string        `db:"file_url" json:"fileUrl"`
	Score        sql.NullInt64 `db:"score" json:"-"`
	AutoScore    sql.NullInt64 `db:"auto_score" json:"-"`
	Comment      string        `db:"comment" json:"comment"`
	SubmittedAt  time.Time     `db:"submitted_at" json:"submittedAt"`
}

// MarshalJSON 把空的老师分数输出成 null，前端据此判断"还没批"。
func (s *SubmissionQueueRow) MarshalJSON() ([]byte, error) {
	type Alias SubmissionQueueRow
	var score *int
	if s.Score.Valid {
		v := int(s.Score.Int64)
		score = &v
	}
	var auto *int
	if s.AutoScore.Valid {
		v := int(s.AutoScore.Int64)
		auto = &v
	}
	return json.Marshal(&struct {
		Score     *int `json:"score"`
		AutoScore *int `json:"autoScore"`
		*Alias
	}{Score: score, AutoScore: auto, Alias: (*Alias)(s)})
}

// ListSubmissionQueue 某作业的全部提交，未批的排在前面（老师先处理待批的）。
func (m *ClassModel) ListSubmissionQueue(assignmentId int64) ([]*SubmissionQueueRow, error) {
	var rows []*SubmissionQueueRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows, `
		SELECT s.id, s.assignment_id, s.user_id, u.username, s.content, s.answers, s.file_url,
		       s.score, s.auto_score, s.comment, s.submitted_at
		  FROM assignment_submissions s
		  JOIN users u ON u.id = s.user_id
		 WHERE s.assignment_id = ?
		 ORDER BY (s.score IS NULL) DESC, s.submitted_at ASC, s.id ASC`, assignmentId)
	return rows, err
}
