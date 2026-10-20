package user

import (
	"context"
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

// 测试用例 3：带真实登录 token → 放行，并把会话信息放入 context
func TestAuthMiddleware_ValidToken(t *testing.T) {
	svcCtx := newTestSvcCtx()

	// 1. 先登录拿到真实 token（会话写入 Redis）
	loginLogic := NewLoginLogic(context.Background(), svcCtx)
	loginResp, err := loginLogic.Login(&LoginRequest{Username: "admin", Password: "123456"})
	if err != nil {
		t.Fatalf("前置准备：登录失败: %v", err)
	}

	// 2. 带有效 token 请求，验证中间件放行且注入会话
	m := NewAuthMiddleware(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/user/info", nil)
	req.Header.Set("Authorization", "Bearer "+loginResp.Token)
	w := httptest.NewRecorder()

	var session model.SessionUser
	m.Handle(func(w http.ResponseWriter, r *http.Request) {
		session, _ = r.Context().Value(authSessionKey).(model.SessionUser)
	})(w, req)

	if session.Username != "admin" {
		t.Fatalf("期望 context 里会话用户是 admin，实际是 %s", session.Username)
	}
	if session.Nickname != "管理员" {
		t.Fatalf("期望 context 里会话昵称是 管理员，实际是 %s", session.Nickname)
	}
}
