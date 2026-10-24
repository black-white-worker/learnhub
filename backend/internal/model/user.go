package model

import (
	"context"
	"errors"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

// User 表示一个用户（对应数据库 users 表）。
type User struct {
	Username string `db:"username"`
	Password string `db:"password"`
	Nickname string `db:"nickname"`
	Role     string `db:"role"`
}

// SessionUser 登录会话信息（登录成功后以 JSON 形式写入 Redis）。
// 与 User 的区别：不包含密码，可安全地存缓存、返回给前端。
type SessionUser struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"` // admin/teacher/student
}

// UserModel 基于 MySQL 的用户数据访问层。
// 说明：原内存版 UserStore 已替换为数据库实现；
// 预置测试账号 admin/123456 由初始化 SQL（backend/deploy/sql/init.sql）写入。
type UserModel struct {
	conn sqlx.SqlConn
}

// NewUserModel 创建用户数据访问层。
func NewUserModel(conn sqlx.SqlConn) *UserModel {
	return &UserModel{conn: conn}
}

// Verify 校验用户名密码是否正确。
// 用户名不存在、密码不匹配或数据库查询出错都返回 false。
func (m *UserModel) Verify(username, password string) bool {
	var u User
	err := m.conn.QueryRowCtx(context.Background(), &u,
		"SELECT username, password, nickname, role FROM users WHERE username = ? LIMIT 1", username)
	if err != nil {
		return false
	}
	return u.Password == password
}

// GetUser 根据用户名返回用户对象（登录增强用，模拟张三开发的第二个功能）。
// 用户不存在时返回 nil。
func (m *UserModel) GetUser(username string) *User {
	var u User
	err := m.conn.QueryRowCtx(context.Background(), &u,
		"SELECT username, password, nickname, role FROM users WHERE username = ? LIMIT 1", username)
	if err != nil {
		return nil
	}
	return &u
}

// Add 新增一个用户（注册功能用，模拟"同事李四"开发的代码）。
// 用户名已存在（唯一索引冲突）时返回"用户名已存在"，避免重复注册。
func (m *UserModel) Add(username, password string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"INSERT INTO users (username, password, nickname) VALUES (?, ?, ?)",
		username, password, username)
	if err != nil {
		// MySQL 唯一索引冲突的错误文本包含 Duplicate entry
		if strings.Contains(err.Error(), "Duplicate entry") {
			return errors.New("用户名已存在")
		}
		return err
	}
	return nil
}

// Delete 按用户名删除用户（测试清理用）。
// 用户不存在时也返回 nil（幂等）。
func (m *UserModel) Delete(username string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"DELETE FROM users WHERE username = ?", username)
	return err
}

// GetIdByUsername 根据用户名查用户 id（班级模块用）。
func (m *UserModel) GetIdByUsername(username string) (int64, error) {
	var id int64
	err := m.conn.QueryRowCtx(context.Background(), &id,
		"SELECT id FROM users WHERE username = ? LIMIT 1", username)
	return id, err
}

// ListAll 管理员用：返回所有用户
func (m *UserModel) ListAll() ([]*User, error) {
	var users []*User
	err := m.conn.QueryRowsCtx(context.Background(), &users,
		"SELECT username, password, nickname, role FROM users ORDER BY id")
	return users, err
}

// UpdateRole 修改用户角色（管理员用）
func (m *UserModel) UpdateRole(username, role string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE users SET role = ? WHERE username = ?", role, username)
	return err
}
