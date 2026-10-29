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
	Id       int64  `db:"id" json:"id"`
	Username string `db:"username" json:"username"`
	Password string `db:"password" json:"-"` // 密码永不下发
	Nickname string `db:"nickname" json:"nickname"`
	Role     string `db:"role" json:"role"`
	// MustChangePassword 初始账号 / 管理员重置密码后置 1，登录成功必须先改密。
	MustChangePassword bool `db:"must_change_password" json:"mustChangePassword"`
	ApplyTeacher       bool `db:"apply_teacher" json:"applyTeacher"`
}

// SessionUser 登录会话信息（登录成功后以 JSON 形式写入 Redis）。
// 与 User 的区别：不包含密码，可安全地存缓存、返回给前端。
//
// Id 是"会话携带身份"的关键：业务接口每请求都要知道用户 id，
// 不带 id 就只能每次 GetIdByUsername 回源一次 MySQL。老会话没有这个字段，
// 反序列化后为 0，调用方会回落到数据库查询（见 class.CurrentUser），
// 所以滚动发布不会把在线用户踢掉。
type SessionUser struct {
	Id       int64  `json:"id,omitempty"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"` // admin/teacher/student
}

// UserModel 基于 MySQL 的用户数据访问层。
// 说明：原内存版 UserStore 已替换为数据库实现；
// 预置测试账号 admin/123456 由 Flyway 迁移 V1__create_users.sql 写入。
type UserModel struct {
	conn sqlx.SqlConn
}

// NewUserModel 创建用户数据访问层。
func NewUserModel(conn sqlx.SqlConn) *UserModel {
	return &UserModel{conn: conn}
}

// Verify 校验用户名密码是否正确。
// 密码统一 bcrypt 哈希存储。为兼容历史明文数据（V1 迁移预置的 admin/123456），
// 若库中密码不以 "$2" 开头则按明文比对；上层登录成功后会自动把明文升级为 bcrypt。
func (m *UserModel) Verify(username, password string) bool {
	var u User
	err := m.conn.QueryRowCtx(context.Background(), &u,
		"SELECT id, username, password, nickname, role, must_change_password, apply_teacher FROM users WHERE username = ? LIMIT 1", username)
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
		"SELECT id, username, password, nickname, role, must_change_password, apply_teacher FROM users WHERE username = ? LIMIT 1", username)
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
		"SELECT id, username, password, nickname, role, must_change_password, apply_teacher FROM users ORDER BY id")
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

// UpdateNickname 管理员修改用户昵称
func (m *UserModel) UpdateNickname(username, nickname string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE users SET nickname = ? WHERE username = ?", nickname, username)
	return err
}

// ResetPassword 管理员重置密码（bcrypt 哈希后落库）。
// 同时把 must_change_password 置 1：管理员代设的密码是临时口令，
// 用户下次登录必须自己改掉，否则"重置成 123456"会一直是弱口令。
func (m *UserModel) ResetPassword(username, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}
	_, err = m.conn.ExecCtx(context.Background(),
		"UPDATE users SET password = ?, must_change_password = 1 WHERE username = ?", string(hash), username)
	return err
}

// ChangePassword 用户自己修改密码：bcrypt 哈希落库并清除"必须改密"标记。
func (m *UserModel) ChangePassword(username, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return errors.New("密码加密失败")
	}
	_, err = m.conn.ExecCtx(context.Background(),
		"UPDATE users SET password = ?, must_change_password = 0 WHERE username = ?", string(hash), username)
	return err
}

// ClearApplyTeacher 审批通过/拒绝时清除申请标记
func (m *UserModel) ClearApplyTeacher(username string) error {
	_, err := m.conn.ExecCtx(context.Background(),
		"UPDATE users SET apply_teacher = 0 WHERE username = ?", username)
	return err
}
