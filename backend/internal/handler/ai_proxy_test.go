package handler

// AI 反向代理的单测：转发前必须把客户端能伪造的身份头换成 Go 自己解出来的可信值，
// 否则任何人直连 ai-service（或带着假 X-User-Id 打网关）就能冒充别的用户 ——
// 答疑会话、AI 出题都是按用户隔离的。
//
// 用例不依赖 MySQL/Redis：只构造 Config + 一个假的 ai-service 目标。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/config"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/aiservice"
	"learningplatform/internal/svc"
)

// fakeAIServer 记录代理转发过来的请求头。
type fakeAIServer struct {
	server *httptest.Server
	header http.Header
	path   string
	method string
}

func newFakeAI(t *testing.T, status int, body string) *fakeAIServer {
	t.Helper()
	got := &fakeAIServer{}
	got.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.path = r.URL.Path
		got.method = r.Method
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(got.server.Close)
	return got
}

func newProxySvcCtx(target string, token string) *svc.ServiceContext {
	c := config.Config{}
	c.AiService.Url = target
	c.AiService.InternalToken = token
	// AI 客户端和 Config 一起装配，和 NewServiceContext 的做法保持一致
	sc := &svc.ServiceContext{Config: c}
	sc.AI = aiservice.New(c.AiService)
	return sc
}

// loginAs 把请求 context 装成"鉴权中间件已经放行"的样子。
func loginAs(r *http.Request, id int64, username, role string) *http.Request {
	session := model.SessionUser{Id: id, Username: username, Role: role}
	return r.WithContext(context.WithValue(r.Context(), user.AuthSessionKey, session))
}

// 用例：客户端伪造的 X-User-Id / X-Role 必须被换成 JWT 解出来的可信值
func TestAIProxy_StripsForgedIdentityHeaders(t *testing.T) {
	ai := newFakeAI(t, 200, `{"ok":true}`)
	proxy := NewAIProxy(newProxySvcCtx(ai.server.URL, "s3cret"))

	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", strings.NewReader(`{"message":"hi"}`))
	// 攻击面：浏览器直接带这两个头，想让 ai-service 以为是老师在提问
	req.Header.Set("X-User-Id", "999999")
	req.Header.Set("X-Role", "admin")
	req = loginAs(req, 42, "stu001", "student")

	rec := httptest.NewRecorder()
	proxy(rec, req)

	if ai.header == nil {
		t.Fatal("请求没转发到 ai-service")
	}
	if got := ai.header.Get("X-User-Id"); got != "42" {
		t.Fatalf("X-User-Id 应被覆盖成登录用户的 42，实际 %q", got)
	}
	if got := ai.header.Get("X-Role"); got != "student" {
		t.Fatalf("X-Role 应被覆盖成 student，实际 %q", got)
	}
	if got := ai.header.Get("X-Internal-Token"); got != "s3cret" {
		t.Fatalf("内部令牌没注入: %q", got)
	}
	if ai.path != "/api/ai/chat" || ai.method != http.MethodPost {
		t.Fatalf("转发路径/方法不对: %s %s", ai.method, ai.path)
	}
	// SSE：代理要关掉上游缓冲，否则 nginx 会攒完整段回答
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Fatalf("响应头缺 X-Accel-Buffering: no，SSE 会被缓冲")
	}
}

// 用例：客户端伪造的 X-Internal-Token 不能带过去（否则等于替攻击者保管密钥）
func TestAIProxy_StripsClientSuppliedInternalToken(t *testing.T) {
	ai := newFakeAI(t, 200, `{"ok":true}`)
	proxy := NewAIProxy(newProxySvcCtx(ai.server.URL, "s3cret"))

	req := httptest.NewRequest(http.MethodPost, "/api/ai/generate", nil)
	req.Header.Set("X-Internal-Token", "guessed-by-attacker")
	req = loginAs(req, 7, "t01", "teacher")

	proxy(httptest.NewRecorder(), req)

	if got := ai.header.Get("X-Internal-Token"); got != "s3cret" {
		t.Fatalf("客户端伪造的令牌没被覆盖，实际 %q", got)
	}
}

// 用例：未配置（Url 或 InternalToken 为空）时统一 503 + JSON 信封，不发起任何转发
func TestAIProxy_NotConfiguredReturns503(t *testing.T) {
	ai := newFakeAI(t, 200, `{"ok":true}`)
	cases := []struct {
		name  string
		url   string
		token string
	}{
		{"两者都空", "", ""},
		{"只有地址", ai.server.URL, ""},
		{"只有令牌", "", "tk"},
		{"地址是垃圾", "://bad url", "tk"},
	}
	for _, c := range cases {
		handler := NewAIProxy(newProxySvcCtx(c.url, c.token))
		rec := httptest.NewRecorder()
		req := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil), 1, "u", "teacher")
		handler(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("%s：期望 503，实际 %d", c.name, rec.Code)
		}
		var env map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
			t.Fatalf("%s：响应不是 JSON 信封: %s", c.name, rec.Body.String())
		}
		if env["code"] != float64(http.StatusServiceUnavailable) {
			t.Fatalf("%s：信封 code 应为 503，实际 %v（body=%s）", c.name, env["code"], rec.Body.String())
		}
		msg, _ := env["message"].(string)
		if !strings.Contains(msg, "AI") {
			t.Fatalf("%s：文案应说明是 AI 功能问题，实际 %q", c.name, msg)
		}
	}
	// 兜底：上面这些配置都不该真的打到 ai-service
	if ai.path != "" {
		t.Fatalf("未配置时不应转发，实际收到了 %s", ai.path)
	}
}

// 用例：ai-service 连不上时也要输出 JSON 信封（默认是裸 502 HTML，前端拦截器读不到 message）
func TestAIProxy_UpstreamDownReturnsJSON503(t *testing.T) {
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := dead.URL
	dead.Close() // 立刻关掉，制造 connection refused

	proxy := NewAIProxy(newProxySvcCtx(addr, "tk"))
	req := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil), 1, "u", "teacher")
	rec := httptest.NewRecorder()
	proxy(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("期望 503，实际 %d：%s", rec.Code, rec.Body.String())
	}
	var env map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("响应不是 JSON 信封: %s", rec.Body.String())
	}
	if msg, _ := env["message"].(string); !strings.Contains(msg, "稍后重试") {
		t.Fatalf("文案应引导稍后重试，实际 %q", msg)
	}
}

// 用例：ai-service 的 4xx/5xx 原样透传状态码，前端据此区分"额度用尽"和"服务挂了"
func TestAIProxy_PassesUpstreamStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusTooManyRequests, http.StatusInternalServerError} {
		ai := newFakeAI(t, status, `{"detail":"额度用尽"}`)
		proxy := NewAIProxy(newProxySvcCtx(ai.server.URL, "tk"))
		req := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil), 1, "u", "teacher")
		rec := httptest.NewRecorder()
		proxy(rec, req)
		if rec.Code != status {
			t.Fatalf("期望透传 %d，实际 %d", status, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "额度用尽") {
			t.Fatalf("上游错误体应原样带回: %s", rec.Body.String())
		}
	}
}

// 用例：没登录（context 里没有会话）时不注入身份头，Python 侧会拒掉
func TestAIProxy_NoSessionSendsNoIdentity(t *testing.T) {
	ai := newFakeAI(t, 401, `{"detail":"missing X-User-Id"}`)
	proxy := NewAIProxy(newProxySvcCtx(ai.server.URL, "tk"))

	req := httptest.NewRequest(http.MethodGet, "/api/ai/session/history?course_id=1", nil)
	req.Header.Set("X-User-Id", "12345") // 伪造，但没有会话，覆盖不成也删不掉？—— 应当被删除
	rec := httptest.NewRecorder()
	proxy(rec, req)

	if got := ai.header.Get("X-User-Id"); got != "" {
		t.Fatalf("未登录时不能把客户端的 X-User-Id 透传过去，实际 %q", got)
	}
	if got := ai.header.Get("X-Internal-Token"); got != "tk" {
		t.Fatalf("内部令牌仍要带上（由 Python 侧决定拒绝）: %q", got)
	}
}

// 用例：/api/ai/status 由 Go 自己回答，配置齐不齐都能给出明确原因
func TestAIStatusHandler(t *testing.T) {
	// 未配置
	svcCtx := newProxySvcCtx("", "")
	rec := httptest.NewRecorder()
	aiStatusHandler(svcCtx)(rec, httptest.NewRequest(http.MethodGet, "/api/ai/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("状态接口本身不该 503，实际 %d", rec.Code)
	}
	var off map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &off); err != nil {
		t.Fatalf("响应不是 JSON: %s", rec.Body.String())
	}
	if off["enabled"] != false {
		t.Fatalf("未配置时 enabled 应为 false，实际 %v", off["enabled"])
	}
	if reason, _ := off["reason"].(string); reason == "" {
		t.Fatal("未配置时必须给出原因，前端才能把按钮置灰并说明")
	}

	// 已配置
	rec = httptest.NewRecorder()
	aiStatusHandler(newProxySvcCtx("http://ai-service:8000", "tk"))(rec, httptest.NewRequest(http.MethodGet, "/api/ai/status", nil))
	var on map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &on); err != nil {
		t.Fatalf("响应不是 JSON: %s", rec.Body.String())
	}
	if on["enabled"] != true {
		t.Fatalf("配置齐全时 enabled 应为 true，实际 %v", on["enabled"])
	}
	feats, ok := on["features"].([]interface{})
	if !ok || len(feats) != 3 {
		t.Fatalf("features 应列出 3 个能力，实际 %v", on["features"])
	}
	joined := fmt.Sprint(feats)
	for _, want := range []string{"chat", "generate", "grade"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("features 缺 %s，实际 %v", want, feats)
		}
	}
}

// 用例：知识库覆盖度接口在白名单里，且只暴露 GET（不能让浏览器写知识库）
func TestAIRoutes_IncludesKbStatusReadOnly(t *testing.T) {
	found := false
	for _, r := range aiRoutes {
		if strings.HasPrefix(r.Path, "/api/ai/kb/") {
			if r.Method != http.MethodGet {
				t.Fatalf("知识库接口不应暴露写方法：%+v", r)
			}
			if r.Path == "/api/ai/kb/status" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("代理白名单缺少 GET /api/ai/kb/status，覆盖度视图会 404")
	}
}

// 用例：普通 JSON 接口（中文专业出题）等几十秒才写响应头，不能被 SSE 的响应头超时掐断。
//
// 线上故障复现：ai_proxy.go 曾给所有 /api/ai/* 统一设 ResponseHeaderTimeout=10s，
// 而 FastAPI 要等「中文资料检索 + 大模型生成」跑完才发第一个响应头，实测 50~120s，
// 于是每次出题都在第 10 秒返回 503「AI 服务暂时不可用」。
// 这里把 SSE 头超时压到 50ms、上游故意 sleep 150ms，快速验证两条链路已分流。
func TestAIProxy_SlowJSONUpstreamIsNotCutByHeaderTimeout(t *testing.T) {
	ai := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond) // 模拟「检索 + LLM」期间一个响应头都不发
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"questions":[]}`))
	}))
	defer ai.Close()
	svcCtx := newProxySvcCtx(ai.URL, "tk")

	// 普通 JSON：即使 SSE 头超时只有 50ms，也必须等上游跑完并原样返回 200
	jsonProxy := newAIProxy(svcCtx, 50*time.Millisecond, 2*time.Second)
	jsonReq := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/generate", nil), 1, "t", "teacher")
	jsonReq.Header.Set("Accept", "application/json")
	jsonRec := httptest.NewRecorder()
	jsonProxy(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("普通 JSON 出题不该被响应头超时掐断，实际 %d：%s", jsonRec.Code, jsonRec.Body.String())
	}
	if !strings.Contains(jsonRec.Body.String(), "questions") {
		t.Fatalf("上游响应体没透传: %s", jsonRec.Body.String())
	}

	// SSE：上游挂了仍要快速失败（只等响应头，不能无限挂着）
	sseReq := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil), 1, "t", "teacher")
	sseReq.Header.Set("Accept", "text/event-stream")
	sseRec := httptest.NewRecorder()
	jsonProxy(sseRec, sseReq)
	if sseRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("SSE 上游不给响应头应快速 503，实际 %d", sseRec.Code)
	}

	// 非 SSE 也不是完全不设防：整体超时到点仍会结束请求
	tight := newAIProxy(svcCtx, 50*time.Millisecond, 40*time.Millisecond)
	tightReq := loginAs(httptest.NewRequest(http.MethodPost, "/api/ai/generate", nil), 1, "t", "teacher")
	tightReq.Header.Set("Accept", "application/json")
	tightRec := httptest.NewRecorder()
	tight(tightRec, tightReq)
	if tightRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("超过整体超时仍应 503，实际 %d", tightRec.Code)
	}
}

// 用例：各层超时必须由外向内逐级放宽，前端才会先给出友好提示而不是收到 Go 的 503
func TestAITimeoutLayering(t *testing.T) {
	if !(sseHeaderTimeout < aiJSONTimeout && aiJSONTimeout < aiRouteTimeout) {
		t.Fatalf("超时层次颠倒：SSE=%v JSON=%v 路由=%v", sseHeaderTimeout, aiJSONTimeout, aiRouteTimeout)
	}
	if aiJSONTimeout <= 150*time.Second {
		t.Fatalf("代理整体超时必须大于前端 axios 的 150s，实际 %v", aiJSONTimeout)
	}
}

// 用例：批改入口不能出现在浏览器代理白名单里（载荷必须由 Go 组装）
func TestAIRoutes_DoNotProxyGradeEndpoints(t *testing.T) {
	for _, r := range aiRoutes {
		if strings.Contains(r.Path, "grade") || strings.Contains(r.Path, "/task") {
			t.Fatalf("批改相关接口不该由浏览器直连： %+v", r)
		}
	}
	// 反过来，前端功能都要在表里（status 由 Go 自己答，不在这张表）
	want := map[string]bool{"/api/ai/chat": false, "/api/ai/generate": false, "/api/ai/session/history": false, "/api/ai/session": false}
	for _, r := range aiRoutes {
		if _, ok := want[r.Path]; ok {
			want[r.Path] = true
		}
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("代理表缺少 %s，前端对应功能会 404", path)
		}
	}
}
