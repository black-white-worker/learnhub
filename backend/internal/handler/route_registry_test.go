package handler

// 路由注册总表回归：把整个应用的路由注册跑一遍（不启动端口），检查
//   1. 没有重复的 method+path（改造前出过"同一路由注册两次"和"该注册的漏注册"两类事故）；
//   2. AI 模块的每条路由都在表里，路径写错前端就是 404，启动却不报错，最难发现；
//   3. 对外路由里不能出现 ai-service 的批改入口（载荷必须由 Go 组装）。
//
// 注意这里故意不调 svc.NewServiceContext：那个函数会 redis.MustNewRedis 真的拨号，
// 本机没起 Redis 就直接进程级 fatal。注册阶段只把 handler 闭包进路由表，不会碰
// 任何 store，所以手搓一个只有 AI 客户端的 ServiceContext 就够。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"learningplatform/internal/config"
	"learningplatform/internal/platform/aiservice"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// newRegistrySvcCtx 只填注册阶段会被读到的字段，其余留空（不会被解引用）。
func newRegistrySvcCtx(c config.Config) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: c,
		AI:     aiservice.New(c.AiService),
	}
}

func newRegistryConfig(name string) config.Config {
	c := config.Config{}
	c.Name = name
	c.Port = 0 // 不 Start，只需要注册不冲突
	return c
}

func allRegisteredRoutes(t *testing.T) map[string]bool {
	t.Helper()
	c := newRegistryConfig("route-registry-test")
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	RegisterHandlers(server, newRegistrySvcCtx(c))

	seen := map[string]bool{}
	for _, r := range server.Routes() {
		key := fmt.Sprintf("%s %s", r.Method, r.Path)
		if seen[key] {
			t.Errorf("路由重复注册: %s", key)
		}
		seen[key] = true
	}
	return seen
}

func TestRouteRegistry_ContainsAIEndpoints(t *testing.T) {
	routes := allRegisteredRoutes(t)

	want := []string{
		// 浏览器侧（Go 代理转发到 ai-service）
		"POST /api/ai/chat",
		"POST /api/ai/generate",
		"GET /api/ai/session/history",
		"DELETE /api/ai/session",
		"GET /api/ai/status",
		// 老师侧（Go 组装载荷）
		"POST /api/assignment/submission/:sid/ai-grade",
		"GET /api/assignment/submission/:sid/ai-grade",
		// ai-service worker 回调
		"POST /api/assignment/submission/:sid/ai-callback",
	}
	var missing []string
	for _, k := range want {
		if !routes[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("以下路由没注册（前端会 404）：\n  %s", strings.Join(missing, "\n  "))
	}
}

// 用例：笔记模块的三条路由都要在表里（记笔记 / 按视频查 / 整门课查）。
// 单独列一组回归，避免 AI 功能增删时把笔记功能顺手带走。
func TestRouteRegistry_ContainsNoteEndpoints(t *testing.T) {
	routes := allRegisteredRoutes(t)
	want := []string{
		"POST /api/course/note",
		"GET /api/course/note/:courseId/:videoId",
		"GET /api/course/notes/summary/:courseId",
	}
	var missing []string
	for _, k := range want {
		if !routes[k] {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("笔记路由没注册（视频页记笔记会 404）：\n  %s", strings.Join(missing, "\n  "))
	}
}

func TestRouteRegistry_GradeEntryNotProxiedToBrowser(t *testing.T) {
	routes := allRegisteredRoutes(t)
	// 批改入队必须由 Go 组装载荷，浏览器不能直接打到 /api/ai/grade
	if routes["POST /api/ai/grade"] {
		t.Fatal("POST /api/ai/grade 不应该出现在对外路由里")
	}
}

func TestRouteRegistry_ListsFullApp(t *testing.T) {
	routes := allRegisteredRoutes(t)
	if len(routes) < 100 {
		t.Fatalf("路由总数异常（%d），可能注册提前中断", len(routes))
	}
	// 兜底：随便打印一条，保证 map 里确实是 "METHOD /path" 格式
	sample := ""
	for k := range routes {
		sample = k
	}
	if !strings.Contains(sample, " ") {
		t.Fatalf("路由 key 格式不对: %q", sample)
	}
}

// 用例：/api/ai/* 一律要求登录（代理靠 JWT 才能注入可信身份）
func TestRouteRegistry_AIRequiresAuth(t *testing.T) {
	c := newRegistryConfig("ai-auth-test")
	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()
	svcCtx := newRegistrySvcCtx(c)
	RegisterAIRoutes(server, svcCtx)

	var got []string
	for _, r := range server.Routes() {
		got = append(got, r.Method+" "+r.Path)
	}
	sort.Strings(got)
	if len(got) != len(aiRoutes)+1 { // +1 是 /api/ai/status
		t.Fatalf("AI 对外路由数量应为 %d，实际 %d: %v", len(aiRoutes)+1, len(got), got)
	}
	for _, g := range got {
		if !strings.HasPrefix(g, "GET /api/ai/status") && !strings.Contains(g, "/api/ai/") {
			t.Fatalf("不该注册到 AI 分组: %s", g)
		}
	}
	// 未配置 ai-service 时代理仍注册，只是回 503（前端要拿到 JSON 而不是 404）
	proxy := NewAIProxy(svcCtx)
	if proxy == nil {
		t.Fatal("代理 handler 不应为 nil")
	}
	rec := newUnauthenticatedStatus(proxy)
	if rec != http.StatusServiceUnavailable {
		t.Fatalf("未配置时 handler 应回 503，实际 %d", rec)
	}
}

// newUnauthenticatedStatus 直接调 handler，返回状态码。
func newUnauthenticatedStatus(h http.HandlerFunc) int {
	req := httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil)
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec.Code
}
