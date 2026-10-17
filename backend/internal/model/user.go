package model

import (
	"errors"
)

// User 表示一个用户。
// 当前是教学版：先放在内存里，后续接 MySQL 时替换为数据库表。
type User struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// UserStore 内存用户存储。
// 说明：现在只预置了一个测试账号 admin/123456；
// "注册功能"分支（模拟另一个同事）会往这里添加"新增用户"的方法。
type UserStore struct {
	users map[string]*User
}

// NewUserStore 创建用户存储并预置测试账号。
func NewUserStore() *UserStore {
	store := &UserStore{
		users: make(map[string]*User),
	}
	// 预置测试账号：admin / 123456
	store.users["admin"] = &User{Username: "admin", Password: "123456"}
	return store
}

// Verify 校验用户名密码是否正确。
// 用户名不存在或密码不匹配都返回 false。
func (s *UserStore) Verify(username, password string) bool {
	user, ok := s.users[username]
	if !ok {
		return false
	}
	return user.Password == password
}

// GetUser 根据用户名返回用户对象（登录增强用，模拟张三开发的第二个功能）。
// 用户不存在时返回 nil。
func (s *UserStore) GetUser(username string) *User {
	return s.users[username]
}

// Add 新增一个用户（注册功能用，模拟"同事李四"开发的代码）。
// 用户名已存在时返回错误，避免重复注册。
func (s *UserStore) Add(username, password string) error {
	if _, ok := s.users[username]; ok {
		return errors.New("用户名已存在")
	}
	s.users[username] = &User{Username: username, Password: password}
	return nil
}
