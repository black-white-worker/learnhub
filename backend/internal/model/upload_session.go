package model

import (
	"context"
	"time"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// UploadSessionRow 分片上传会话表（对应 V18 建的 upload_sessions）。
type UploadSessionRow struct {
	Id          int64     `db:"id" json:"id"`
	UploadID    string    `db:"upload_id" json:"uploadId"`
	CourseId    int64     `db:"course_id" json:"courseId"`
	ObjectName  string    `db:"object_name" json:"objectName"`
	FileName    string    `db:"file_name" json:"fileName"`
	FileSize    int64     `db:"file_size" json:"fileSize"`
	ContentType string    `db:"content_type" json:"contentType"`
	Status      string    `db:"status" json:"status"` // init/completed/aborted
	ExpireAt    time.Time `db:"expire_at" json:"expireAt"`
	CreatedBy   int64     `db:"created_by" json:"createdBy"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	CompletedAt *time.Time `db:"completed_at" json:"completedAt,omitempty"`
}

// UploadSessionModel upload_sessions 表 DAO。
type UploadSessionModel struct {
	conn sqlx.SqlConn
}

func NewUploadSessionModel(conn sqlx.SqlConn) *UploadSessionModel {
	return &UploadSessionModel{conn: conn}
}

// Insert 新建会话，返回自增 id。
func (m *UploadSessionModel) Insert(s *UploadSessionRow) (int64, error) {
	res, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO upload_sessions (upload_id, course_id, object_name, file_name, file_size, content_type, status, expire_at, created_by)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		s.UploadID, s.CourseId, s.ObjectName, s.FileName, s.FileSize, s.ContentType, s.Status, s.ExpireAt, s.CreatedBy)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetByUploadID 按 MinIO 返回的 upload_id 查会话。
func (m *UploadSessionModel) GetByUploadID(uploadID string) (*UploadSessionRow, error) {
	var r UploadSessionRow
	err := m.conn.QueryRowCtx(context.Background(), &r,
		`SELECT id, upload_id, course_id, object_name, file_name, file_size, content_type, status, expire_at, created_by, created_at, completed_at
		 FROM upload_sessions WHERE upload_id=?`, uploadID)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// MarkCompleted 标记合并完成。
func (m *UploadSessionModel) MarkCompleted(uploadID string, completedAt time.Time) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE upload_sessions SET status='completed', completed_at=? WHERE upload_id=?`,
		completedAt, uploadID)
	return err
}

// MarkAborted 标记取消。
func (m *UploadSessionModel) MarkAborted(uploadID string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`UPDATE upload_sessions SET status='aborted' WHERE upload_id=?`, uploadID)
	return err
}
