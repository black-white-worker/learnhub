package model

import (
	"context"
	"errors"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"golang.org/x/crypto/bcrypt"
)

// User 表示一个用户（对应数据库 users 表）。
type User struct {
	Username     string `db:"username"`
	Password     string `db:"password"`
	Nickname     string `db:"nickname"`
	Role         string `db:"role"`
	ApplyTeacher bool   `db:"apply_teacher"`
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
// 密码统一 bcrypt 哈希存储。为兼容历史明文数据（init.sql 里的 admin/123456），
// 若库中密码不以 "$2" 开头则按明文比对；上层登录成功后会自动把明文升级为 bcrypt。
func (m *UserModel) Verify(username, password string) bool {
	var u User
	err := m.conn.QueryRowCtx(context.Background(), &u,
		"SELECT username, password, nickname, role, apply_teacher FROM users WHERE username = ? LIMIT 1", username)
	if err != nil {
		return false
	}
	if strings.HasPrefix(u.Password, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)) == nil
	}
	return u.Password == password
}

// IsBcrypted 判断库中该用户密码是否已经是 bcrypt 哈希（登录成功后自动升级用）
func (m *UserModel) IsBcrypted(username string) bool {
	var pwd string
	err := m.conn.QueryRowCtx(context.Background(), &pwd,
		"SELECT password FROM users WHERE username = ? LIMIT 1", username)
	if err != nil {
		return false
	}
	return strings.HasPrefix(pwd, "$2")
}

// UpgradePasswordHash 把明文密码用 bcrypt 哈希后写回（老用户首次登录自动调用）
func (m *UserModel) UpgradePasswordHash(username, rawPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(rawPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = m.conn.ExecCtx(context.Background(),
		"UPDATE users SET password = ? WHERE username = ?", string(hash), username)
	return err
}

// GetUser 根据用户名返回用户对象（登录增强用，模拟张三开发的第二个功能）。
// 用户不存在时返回 nil。
func (m *UserModel) GetUser(username string) *User {
	var u User
	err := m.conn.QueryRowCtx(context.Background(), &u,
		"SELECT username, password, nickname, role, apply_teacher FROM users WHERE username = ? LIMIT 1", username)
	if err != nil {
		return nil
	}
	return &u
}

// Add 新增一个用户（注册功能用）。密码在落库前用 bcrypt 哈希，不存明文。
// 用户名已存在（唯一索引冲突）时返回"用户名已存在"。
func (m *UserModel) Add(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}
	_, err = m.conn.ExecCtx(context.Background(),
		"INSERT INTO users (username, password, nickname) VALUES (?, ?, ?)",
		username, string(hash), username)
	if err != nil {
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
		"SELECT username, password, nickname, role, apply_teacher FROM users ORDER BY id")
	return users, err
}

// ApplyTeacher 学生申请成为老师
func (m *UserModel) ApplyTeacher(username string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE users SET apply_teacher = 1 WHERE username = ?", username)
	return err
}

// UpdateRole 修改用户角色（管理员用）
func (m *UserModel) UpdateRole(username, role string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE users SET role = ? WHERE username = ?", role, username)
	return err
}
