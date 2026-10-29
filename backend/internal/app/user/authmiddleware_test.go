package user

// 鉴权中间件的单元测试。
//
// 说明：这里的用例只依赖 Redis（会话存储），不依赖 MySQL——
// 改造前的第三个用例走了"真登录"，需要预置账号 + 数据库，本地没起容器就整包失败。
// 现在直接把会话写进 Redis，测的是中间件本身的行为，边界更清晰。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"learningplatform/internal/model"
)

// 测试用例 1：不带 Authorization 头 → 401 拒绝，不放行
func TestAuthMiddleware_NoToken(t *testing.T) {
	m := NewAuthMiddleware(newTestSvcCtx())
	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	w := httptest.NewRecorder()

	called := false
	m.Handle(func(w http.ResponseWriter, r *http.Request) { called = true })(w, req)

	if called {
		t.Fatal("期望无 token 时不放行业务逻辑")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望状态码 401，实际是 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "未登录") {
		t.Fatalf("期望返回未登录提示，实际是 %s", w.Body.String())
	}
}

// 测试用例 2：带一个不存在的 token → 401 拒绝（Redis 查不到会话）
func TestAuthMiddleware_InvalidToken(t *testing.T) {
	m := NewAuthMiddleware(newTestSvcCtx())
	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()

	called := false
	m.Handle(func(w http.ResponseWriter, r *http.Request) { called = true })(w, req)

	if called {
		t.Fatal("期望无效 token 时不放行业务逻辑")
	}
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("期望状态码 401，实际是 %d", w.Code)
	}
}

// 测试用例 3：带有效会话 → 放行，并把会话信息放入 context。
func TestAuthMiddleware_ValidToken(t *testing.T) {
	svcCtx := newTestSvcCtx()

	// 1. 直接把会话写进 Redis（等价于登录成功后的状态，省掉 MySQL 依赖）
	token := "unit-test-token-" + strings.Repeat("a", 8)
	session := model.SessionUser{Username: "admin", Nickname: "管理员", Role: "admin"}
	data, err := json.Marshal(session)
	if err != nil {
		t.Fatalf("序列化会话失败: %v", err)
	}
	if err := svcCtx.Redis.Setex(SessionKeyPrefix+token, string(data), 3600); err != nil {
		t.Fatalf("写入 Redis 会话失败（Redis 未启动？）: %v", err)
	}

	// 2. 带有效 token 请求，验证中间件放行且注入会话
	m := NewAuthMiddleware(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	var got *model.SessionUser
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		got = CurrentUser(r)
	})(w, req)

	if got == nil {
		t.Fatal("期望中间件把会话放进 context")
	}
	if got.Username != "admin" || got.Role != "admin" {
		t.Fatalf("context 里的会话不对: %+v", got)
	}
}

// 测试用例 4：可选登录——没有 token 也放行，但 CurrentUser 为空。
// 公开接口（课程详情）靠它区分"登录了吗"，同时不能把游客挡在门外。
func TestAuthMiddleware_OptionalNoToken(t *testing.T) {
	m := NewAuthMiddleware(newTestSvcCtx())
	req := httptest.NewRequest(http.MethodGet, "/api/course/detail/1", nil)
	w := httptest.NewRecorder()

	called := false
	var userInCtx *model.SessionUser
	m.HandleOptional(func(w http.ResponseWriter, r *http.Request) {
		called = true
		userInCtx = CurrentUser(r)
	})(w, req)

	if !called {
		t.Fatal("可选登录在没有 token 时也必须放行")
	}
	if w.Code == http.StatusUnauthorized {
		t.Fatal("可选登录不应返回 401")
	}
	if userInCtx != nil {
		t.Fatalf("没有 token 时不应注入用户: %+v", userInCtx)
	}
}

// 测试用例 5：可选登录——带有效 token 时识别用户。
func TestAuthMiddleware_OptionalValidToken(t *testing.T) {
	svcCtx := newTestSvcCtx()
	token := "optional-token-" + strings.Repeat("b", 8)
	data, _ := json.Marshal(model.SessionUser{Username: "admin", Nickname: "管理员", Role: "admin"})
	if err := svcCtx.Redis.Setex(SessionKeyPrefix+token, string(data), 3600); err != nil {
		t.Fatalf("写入 Redis 会话失败: %v", err)
	}

	m := NewAuthMiddleware(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/course/detail/1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	var got *model.SessionUser
	m.HandleOptional(func(w http.ResponseWriter, r *http.Request) {
		got = CurrentUser(r)
	})(w, req)

	if got == nil || got.Username != "admin" {
		t.Fatalf("有效 token 应被识别，实际: %+v", got)
	}
}
