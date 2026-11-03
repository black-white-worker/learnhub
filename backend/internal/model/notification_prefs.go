package model

import (
	"context"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// NotificationPrefRow 映射 notification_preferences 表的一行。
// db tag 对应列名；json tag 决定序列化后的字段名（与前端约定）。
type NotificationPrefRow struct {
	Id      int64  `db:"id" json:"id"`
	UserID  int64  `db:"user_id" json:"userId"`
	Channel string `db:"channel" json:"channel"` // inapp=站内，email=邮件
	Enabled int    `db:"enabled" json:"enabled"` // 1=开启，0=关闭
}

// NotificationPrefModel 通知偏好的数据访问对象（DAO）。
// 表由 Flyway 迁移 V23__notification_preferences.sql 创建，唯一键 (user_id, channel)。
type NotificationPrefModel struct {
	conn sqlx.SqlConn
}

// NewNotificationPrefModel 构造偏好 DAO，复用全局 MySQL 连接。
func NewNotificationPrefModel(conn sqlx.SqlConn) *NotificationPrefModel {
	return &NotificationPrefModel{conn: conn}
}

// ListByUser 查询某用户在所有渠道上的通知偏好。
func (m *NotificationPrefModel) ListByUser(userID int64) ([]NotificationPrefRow, error) {
	var rows []NotificationPrefRow
	err := m.conn.QueryRowsCtx(context.Background(), &rows,
		`SELECT id, user_id, channel, enabled FROM notification_preferences WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// Upsert 按 (user_id, channel) 唯一键写入偏好：已存在则更新 enabled，不存在则插入。
// 配合表里的唯一键保证幂等，业务侧可放心重复调用。
func (m *NotificationPrefModel) Upsert(userID int64, channel string, enabled int) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO notification_preferences (user_id, channel, enabled) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE enabled = VALUES(enabled)`,
		userID, channel, enabled)
	return err
}
