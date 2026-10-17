package model

// User 表示一个用户。
// 当前是教学版：先放在内存里，后续接 MySQL 时替换为数据库表。
type User struct {
	Username string
	Password string
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
