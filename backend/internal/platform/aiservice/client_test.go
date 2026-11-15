package aiservice

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"learningplatform/internal/config"
)

// capturedRequest 假 ai-service 收到的请求，测试用它断言头和载荷。
type capturedRequest struct {
	method     string
	path       string
	header     http.Header
	body       map[string]interface{}
	rawBody    string
	capturedAt *testing.T
}

// newTestClient 起一个假 ai-service，返回客户端和一个装请求的指针。
func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{capturedAt: t}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got.method, got.path, got.rawBody = r.Method, r.URL.Path, string(raw)
		got.header = r.Header.Clone()
		_ = json.Unmarshal(raw, &got.body) // 表单/空体的用例不关心结构化内容
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	// srv.URL 自带 http:// 前缀，正好走「有 scheme 就不补」的分支
	return New(config.AiServiceConf{Url: srv.URL, InternalToken: "test-token"}), got
}

// mustArray 从捕获的 JSON 载荷里取数组字段，取不到就直接 fail。
func (c *capturedRequest) mustArray(t *testing.T, key string) []interface{} {
	t.Helper()
	arr, ok := c.body[key].([]interface{})
	if !ok {
		t.Fatalf("载荷里 %s 不是数组，raw=%s", key, c.rawBody)
	}
	return arr
}

// 用例：配置没齐（Url 或 InternalToken 为空）时一律 ErrDisabled，不发请求
func TestClientDisabled(t *testing.T) {
	cases := []config.AiServiceConf{
		{},
		{Url: "http://ai:8000"},
		{InternalToken: "tk"},
		{Url: "  ", InternalToken: "  tk  "}, // 全空格等同于没配？token 有值、url 空 → 仍然未启用
	}
	for i, c := range cases {
		cl := New(c)
		if cl.Enabled() {
			t.Fatalf("用例 %d 应判定为未启用", i)
		}
		_, err := cl.EnqueueGrade(context.Background(), Actor{}, &GradeRequest{SubmitId: 1, Items: []GradeItem{{QuestionId: 1}}})
		if !errors.Is(err, ErrDisabled) {
			t.Fatalf("用例 %d 期望 ErrDisabled，实际 %v", i, err)
		}
		if _, err := cl.GetGradeTask(context.Background(), Actor{}, "abc"); !errors.Is(err, ErrDisabled) {
			t.Fatalf("用例 %d 查状态期望 ErrDisabled，实际 %v", i, err)
		}
	}
	// 前后空格要能容忍：yaml 里手写成 " http://ai:8000 " 不该判成未配置
	if !New(config.AiServiceConf{Url: "  http://ai:8000  ", InternalToken: "  tk "}).Enabled() {
		t.Fatalf("带空格的合法配置应识别为已启用")
	}

	// 只有空白的一侧才算没配：token 全空格 = 空 = 未启用（防 yaml 里写成 " "）
	if New(config.AiServiceConf{Url: "http://ai:8000", InternalToken: "   "}).Enabled() {
		t.Fatalf("全空格的令牌应判定为未启用")
	}
}

// 用例：入队带上内部令牌与可信身份头，taskId 原样透传
func TestEnqueueGradeRequestShape(t *testing.T) {
	cl, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"taskId":"deadbeef","status":"pending","itemCount":2}`))
	})
	req := &GradeRequest{
		SubmitId:     7,
		AssignmentId: 3,
		StudentId:    99,
		TaskId:       "deadbeef",
		Items:        []GradeItem{{QuestionId: 1, MaxScore: 10}, {QuestionId: 2, MaxScore: 5, ScoreRule: "按要点"}},
	}
	receipt, err := cl.EnqueueGrade(context.Background(), Actor{UserId: 5, Role: "teacher"}, req)
	if err != nil {
		t.Fatalf("入队失败: %v", err)
	}
	if receipt.TaskId != "deadbeef" || receipt.Status != "pending" || receipt.ItemCount != 2 {
		t.Fatalf("回执解析不对: %+v", receipt)
	}
	if got := got.header.Get("X-Internal-Token"); got != "test-token" {
		t.Fatalf("内部令牌没带上: %q", got)
	}
	if got := got.header.Get("X-User-Id"); got != "5" {
		t.Fatalf("X-User-Id 应为发起批改的老师 id，实际 %q", got)
	}
	if got := got.header.Get("X-Role"); got != "teacher" {
		t.Fatalf("X-Role 没带上: %q", got)
	}
	if got.path != "/api/ai/grade" {
		t.Fatalf("路径不对: %s", got.path)
	}
	sent := got.body
	if sent["taskId"] != "deadbeef" {
		t.Fatalf("taskId 没透传给 ai-service（Go 先落库再入队要靠它对齐）: %v", sent["taskId"])
	}
	items := got.mustArray(t, "items")
	if len(items) != 2 {
		t.Fatalf("items 应原样带 2 道题，实际 %v", sent["items"])
	}
	// 空的 ScoreRule 不该占位（omitempty）
	first := items[0].(map[string]interface{})
	if _, has := first["scoreRule"]; has {
		t.Fatalf("空 scoreRule 不应出现在载荷里: %v", first)
	}
}

// 用例：ai-service 没返回 taskId 视为不可用，不能返回空任务号让前端拿不到句柄
func TestEnqueueGradeMissingTaskId(t *testing.T) {
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"pending"}`))
	})
	_, err := cl.EnqueueGrade(context.Background(), Actor{UserId: 1}, &GradeRequest{SubmitId: 1, Items: []GradeItem{{QuestionId: 1}}})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("缺 taskId 应报 ErrUnavailable，实际 %v", err)
	}
}

// 用例：没有主观题载荷时不要发请求（省一次网络往返，也给前端明确文案）
func TestEnqueueGradeRejectsEmptyItems(t *testing.T) {
	called := false
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	if _, err := cl.EnqueueGrade(context.Background(), Actor{UserId: 1}, &GradeRequest{SubmitId: 1}); err == nil {
		t.Fatalf("items 为空应报错")
	}
	if called {
		t.Fatalf("items 为空时不该发请求")
	}
}

// 用例：5xx 归一成 ErrUnavailable；连不上同理
func TestEnqueueGradeUnavailable(t *testing.T) {
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"detail":"boom"}`, http.StatusInternalServerError)
	})
	if _, err := cl.EnqueueGrade(context.Background(), Actor{UserId: 1}, &GradeRequest{SubmitId: 1, Items: []GradeItem{{QuestionId: 1}}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("500 应报 ErrUnavailable，实际 %v", err)
	}

	// 关掉服务再调：网络层错误也必须是 ErrUnavailable，而不是裸的 dial 错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	addr := srv.URL
	srv.Close()
	dead := New(config.AiServiceConf{Url: addr, InternalToken: "t"})
	if _, err := dead.EnqueueGrade(context.Background(), Actor{UserId: 1}, &GradeRequest{SubmitId: 1, Items: []GradeItem{{QuestionId: 1}}}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("连不上应报 ErrUnavailable，实际 %v", err)
	}
}

// 用例：4xx 是 ai-service 明确拒绝，要把它的 detail 带出来，不能混成「服务不可用」
func TestClient4xxDetailPassthrough(t *testing.T) {
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"detail":"只有教师或管理员可以发起 AI 批改"}`))
	})
	_, err := cl.EnqueueGrade(context.Background(), Actor{UserId: 1, Role: "student"}, &GradeRequest{SubmitId: 1, Items: []GradeItem{{QuestionId: 1}}})
	if err == nil {
		t.Fatalf("403 应报错")
	}
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrDisabled) {
		t.Fatalf("4xx 不该归类为不可用/未启用: %v", err)
	}
	if !strings.Contains(err.Error(), "只有教师或管理员") {
		t.Fatalf("错误信息应带上 ai-service 的 detail，实际 %q", err.Error())
	}
}

// 用例：BaseUrl 归一化 —— 补 scheme、去尾斜杠
func TestClientNormalizeBaseUrl(t *testing.T) {
	if got := New(config.AiServiceConf{Url: "ai-service:8000/", InternalToken: "t"}).base; got != "http://ai-service:8000" {
		t.Fatalf("应补 http:// 并去掉尾斜杠，实际 %q", got)
	}
	if got := New(config.AiServiceConf{Url: "http://ai-service:8000///", InternalToken: "t"}).base; got != "http://ai-service:8000" {
		t.Fatalf("多余尾斜杠应清掉，实际 %q", got)
	}
}

// 用例：查任务状态 —— Python 出来的数字可能是 18 也可能是 18.0，都要能收成 int
func TestGetGradeTaskResultParsing(t *testing.T) {
	cl, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"taskId":"abc123",
			"status":"done",
			"itemCount":2,
			"error":"",
			"result":{
				"score":18.0,
				"maxScore":30,
				"comment":"AI 初评：2 道主观题",
				"parseOk":true,
				"items":[
					{"questionId":1,"score":12.6,"maxScore":20,"comment":"还行","errorPoints":["漏了要点"],"suggestion":"补一句","parseOk":true},
					{"questionId":2,"score":null,"maxScore":10,"comment":"没批出来","errorPoints":[],"suggestion":"","parseOk":false}
				]
			}
		}`))
	})
	st, err := cl.GetGradeTask(context.Background(), Actor{UserId: 5, Role: "teacher"}, "abc123")
	if err != nil {
		t.Fatalf("查状态失败: %v", err)
	}
	if got.path != "/api/ai/task/abc123" {
		t.Fatalf("路径不对: %s", got.path)
	}
	if st.Status != "done" || st.ItemCount != 2 {
		t.Fatalf("基本字段不对: %+v", st)
	}
	if st.Result == nil || st.Result.Score != 18 || st.Result.MaxScore != 30 {
		t.Fatalf("总分应对 18.0 做四舍五入取整，实际 %+v", st.Result)
	}
	if len(st.Result.Items) != 2 {
		t.Fatalf("逐题结果应 2 条，实际 %d", len(st.Result.Items))
	}
	if st.Result.Items[0].Score != 13 {
		t.Fatalf("12.6 应四舍五入成 13，实际 %d", st.Result.Items[0].Score)
	}
	if st.Result.Items[0].QuestionId != 1 || st.Result.Items[0].MaxScore != 20 {
		t.Fatalf("题号/满分没对上: %+v", st.Result.Items[0])
	}
	if len(st.Result.Items[0].ErrorPoints) != 1 || st.Result.Items[0].ErrorPoints[0] != "漏了要点" {
		t.Fatalf("错误点没解出来: %+v", st.Result.Items[0])
	}
	// score 为 null（没批出来的题）收成 0，但 parseOk=false 由调用方判断
	if st.Result.Items[1].Score != 0 || st.Result.Items[1].ParseOk {
		t.Fatalf("未批出的题应为 0 分且 parseOk=false: %+v", st.Result.Items[1])
	}
}

// 用例：任务过期（Redis TTL 到）返回 ErrTaskGone，调用方据此停止轮询
func TestGetGradeTaskGone(t *testing.T) {
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"批改任务不存在或已过期"}`))
	})
	if _, err := cl.GetGradeTask(context.Background(), Actor{UserId: 1}, "gone"); !errors.Is(err, ErrTaskGone) {
		t.Fatalf("404 应报 ErrTaskGone，实际 %v", err)
	}
	if _, err := cl.GetGradeTask(context.Background(), Actor{UserId: 1}, "   "); err == nil {
		t.Fatalf("空 taskId 应直接报错，不发请求")
	}
}

// 用例：result 为 null（还在排队）时不该硬解出零值结果
func TestGetGradeTaskPendingNoResult(t *testing.T) {
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"taskId":"t1","status":"pending","itemCount":1,"result":null,"error":""}`))
	})
	st, err := cl.GetGradeTask(context.Background(), Actor{UserId: 1}, "t1")
	if err != nil {
		t.Fatalf("查状态失败: %v", err)
	}
	if st.Result != nil {
		t.Fatalf("pending 任务的 result 应为 nil，实际 %+v", st.Result)
	}
}

// ===== 知识库索引联动（讲义上传/删除）=====

// 用例：讲义上传后触发灌库 —— 载荷带课程 + 资源 id，身份头可信
func TestIngestCourseRequestShape(t *testing.T) {
	cl, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"course_id":7,"resource_id":9,"accepted":true,"running":true,"reason":""}`))
	})
	receipt, err := cl.IngestCourse(context.Background(), Actor{UserId: 5, Role: "teacher"}, 7, 9)
	if err != nil {
		t.Fatalf("提交灌库失败: %v", err)
	}
	if got.path != "/api/ai/kb/ingest" || got.method != http.MethodPost {
		t.Fatalf("路径/方法不对: %s %s", got.method, got.path)
	}
	if got.header.Get("X-Internal-Token") != "test-token" {
		t.Fatalf("内部令牌没带上")
	}
	if got.body["course_id"].(float64) != 7 || got.body["resource_id"].(float64) != 9 {
		t.Fatalf("载荷不对: %s", got.rawBody)
	}
	if !receipt.Accepted || receipt.CourseId != 7 || receipt.ResourceId != 9 {
		t.Fatalf("回执解析不对: %+v", receipt)
	}
}

// 用例：整门课重灌时不带 resource_id（Python 侧按 None 处理）
func TestIngestCourseWholeCourseOmitsResourceId(t *testing.T) {
	cl, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"course_id":3,"accepted":true,"queued":true,"running":true,"reason":"排队中"}`))
	})
	receipt, err := cl.IngestCourse(context.Background(), Actor{}, 3, 0)
	if err != nil {
		t.Fatalf("提交灌库失败: %v", err)
	}
	if _, has := got.body["resource_id"]; has {
		t.Fatalf("整门课重灌不该带 resource_id: %s", got.rawBody)
	}
	// 忙时排队不丢任务：accepted 仍为 true，queued 让日志能解释「为什么还没入库」
	if !receipt.Accepted || !receipt.Queued {
		t.Fatalf("应识别为已排队: %+v", receipt)
	}
	if receipt.Reason == "" {
		t.Fatalf("排队原因应透传，日志要能解释为什么没立即生效")
	}
}

// 用例：缺课程 id 直接报错，不发请求
func TestIngestCourseRejectsBadCourseId(t *testing.T) {
	called := false
	cl, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) { called = true })
	if _, err := cl.IngestCourse(context.Background(), Actor{}, 0, 1); err == nil {
		t.Fatalf("courseId=0 应报错")
	}
	if called {
		t.Fatalf("参数不合法时不该发请求")
	}
}

// 用例：资源删除后清理索引
func TestPurgeResourceRequestShape(t *testing.T) {
	cl, got := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"course_id":7,"resource_id":9,"chunks_cleared":true,"vectors_cleared":true}`))
	})
	if err := cl.PurgeResource(context.Background(), Actor{UserId: 5, Role: "teacher"}, 7, 9); err != nil {
		t.Fatalf("清理索引失败: %v", err)
	}
	if got.path != "/api/ai/kb/purge" {
		t.Fatalf("路径不对: %s", got.path)
	}
	if got.body["resource_id"].(float64) != 9 {
		t.Fatalf("载荷不对: %s", got.rawBody)
	}
}

// 用例：未配置 AI 时灌库/清理都返回 ErrDisabled，调用方据此静默跳过
func TestKbCallsDisabled(t *testing.T) {
	cl := New(config.AiServiceConf{})
	if _, err := cl.IngestCourse(context.Background(), Actor{}, 1, 2); !errors.Is(err, ErrDisabled) {
		t.Fatalf("应报 ErrDisabled，实际 %v", err)
	}
	if err := cl.PurgeResource(context.Background(), Actor{}, 1, 2); !errors.Is(err, ErrDisabled) {
		t.Fatalf("应报 ErrDisabled，实际 %v", err)
	}
}
