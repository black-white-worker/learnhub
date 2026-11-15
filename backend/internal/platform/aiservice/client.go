// Package aiservice 是 Go 主服务调用 ai-service（FastAPI）的服务间客户端。
//
// 和 handler.NewAIProxy 的分工：
//   - 代理：浏览器发的 /api/ai/* 原样转发（答疑 / 出题这类"只读或产出文本"的接口）；
//   - 本包：**必须由服务端组装载荷**的调用 —— 主观题批改的题干、参考答案、学生作答
//     全部从数据库取，不能让前端自己拼一份 standardAnswer 递上来。
//
// 身份与鉴权：请求带 X-Internal-Token（共享密钥）+ X-User-Id / X-Role（发起批改的老师），
// Python 侧据此校验角色并记录是谁触发的。
package aiservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/config"
)

// ErrDisabled：AiService 没配（Url 或 InternalToken 为空）。
// 调用方据此给出"AI 功能未启用"的文案，而不是把它当成网络故障重试。
var ErrDisabled = errors.New("AI 服务未启用")

// ErrUnavailable：ai-service 连不上 / 5xx。对外统一文案「AI 服务暂时不可用」。
var ErrUnavailable = errors.New("AI 服务暂时不可用")

// 超时口径：
//   - 入队只写 Redis，几毫秒的事，5 秒足够；超了就说明 ai-service 挂了，别拖着请求。
//   - 查任务状态同理。这两个都不等 LLM 跑完（那是 worker 的事）。
const (
	enqueueTimeout = 5 * time.Second
	statusTimeout  = 5 * time.Second
	// 灌库回执：ai-service 只把任务丢进后台线程就返回，正常几十毫秒；
	// 5 秒还没回执说明服务卡住了，不必让上传接口陪着等。
	ingestAckTimeout = 5 * time.Second
	// 清理索引是几条 delete，给 10 秒足够（Mongo 抖动时不至于挂住删除接口太久）。
	purgeTimeout = 10 * time.Second
	// 响应体上限：任务结果里带逐题评语，正常几 KB；1MB 是防御性的天花板
	maxResponseBytes = 1 << 20
)

// Actor 发起调用的用户（老师）。
type Actor struct {
	UserId int64
	Role   string
}

// Client ai-service HTTP 客户端。零值不可用，请用 New()。
type Client struct {
	base  string
	token string
	http  *http.Client
}

// New 从配置建客户端。配置不全时返回的客户端所有方法都直接报 ErrDisabled。
func New(c config.AiServiceConf) *Client {
	base := strings.TrimRight(strings.TrimSpace(c.Url), "/")
	if base != "" && !strings.Contains(base, "://") {
		base = "http://" + base
	}
	return &Client{base: base, token: strings.TrimSpace(c.InternalToken), http: &http.Client{}}
}

// Enabled 配置是否齐全（前端菜单和按钮的可用性由它决定）。
func (cl *Client) Enabled() bool {
	return cl != nil && cl.base != "" && cl.token != ""
}

// GradeItem 一道待批改的主观题（载荷由 Go 从库里组装）。
type GradeItem struct {
	QuestionId     int64  `json:"questionId"`
	Stem           string `json:"stem"`
	StudentAnswer  string `json:"studentAnswer"`
	StandardAnswer string `json:"standardAnswer"`
	MaxScore       int    `json:"maxScore"`
	ScoreRule      string `json:"scoreRule,omitempty"`
}

// GradeRequest 一次批改任务（一份提交的全部主观题）。
type GradeRequest struct {
	SubmitId     int64       `json:"submitId"`
	AssignmentId int64       `json:"assignmentId,omitempty"`
	CourseId     int64       `json:"courseId,omitempty"`
	StudentId    int64       `json:"studentId,omitempty"`
	Items        []GradeItem `json:"items"`
	// TaskId 由调用方先生成：Go 会先按这个 id 落一条 processing 记录，
	// 再入队。顺序反过来，worker 的回调可能比落库先到，结果会被覆盖掉。
	// 留空则由 ai-service 生成。
	TaskId string `json:"taskId,omitempty"`
}

// Receipt 入队回执。
type Receipt struct {
	TaskId    string `json:"taskId"`
	Status    string `json:"status"`
	ItemCount int    `json:"itemCount"`
}

// EnqueueGrade 提交批改任务，立刻返回（不阻塞等 LLM）。
func (cl *Client) EnqueueGrade(ctx context.Context, actor Actor, req *GradeRequest) (*Receipt, error) {
	if !cl.Enabled() {
		return nil, ErrDisabled
	}
	if len(req.Items) == 0 {
		return nil, errors.New("没有需要批改的主观题")
	}
	callCtx, cancel := context.WithTimeout(ctx, enqueueTimeout)
	defer cancel()

	var out struct {
		TaskId    string      `json:"taskId"`
		Status    string      `json:"status"`
		ItemCount json.Number `json:"itemCount"`
	}
	if err := cl.postJSON(callCtx, "/api/ai/grade", actor, req, &out); err != nil {
		return nil, err
	}
	if out.TaskId == "" {
		return nil, fmt.Errorf("%w：批改入队没有返回 taskId", ErrUnavailable)
	}
	return &Receipt{TaskId: out.TaskId, Status: out.Status, ItemCount: int(num(out.ItemCount))}, nil
}

// ===== 知识库灌入（讲义上传/删除后维护 RAG 索引）=====
//
// 这两个调用的意义：「上传了讲义 → 答疑能引用到它」本该是自动的。
// 之前只有 scripts/ingest.py 这条手工通道，老师传完讲义不跑脚本，
// 知识库就是空的，学生一问只能得到「资料中没有相关内容」。

// IngestReceipt 灌库回执。ai-service 立刻返回，真正解析在后台线程里跑，
// 所以这里只代表「已提交」，不代表「已入库」。
type IngestReceipt struct {
	CourseId   int64
	ResourceId int64
	Accepted   bool
	Queued     bool // 同一门课已有任务在跑，本次已排队
	Running    bool
	Reason     string
}

// IngestCourse 触发一门课的灌库；resourceId>0 时只灌这一份讲义。
func (cl *Client) IngestCourse(ctx context.Context, actor Actor, courseId, resourceId int64) (*IngestReceipt, error) {
	if !cl.Enabled() {
		return nil, ErrDisabled
	}
	if courseId <= 0 {
		return nil, errors.New("缺少课程 id")
	}
	body := map[string]interface{}{"course_id": courseId}
	if resourceId > 0 {
		body["resource_id"] = resourceId
	}
	callCtx, cancel := context.WithTimeout(ctx, ingestAckTimeout)
	defer cancel()

	var out struct {
		CourseId   json.Number `json:"course_id"`
		ResourceId json.Number `json:"resource_id"`
		Accepted   bool        `json:"accepted"`
		Queued     bool        `json:"queued"`
		Running    bool        `json:"running"`
		Reason     string      `json:"reason"`
	}
	if err := cl.postJSON(callCtx, "/api/ai/kb/ingest", actor, body, &out); err != nil {
		return nil, err
	}
	return &IngestReceipt{
		CourseId:   int64(num(out.CourseId)),
		ResourceId: int64(num(out.ResourceId)),
		Accepted:   out.Accepted,
		Queued:     out.Queued,
		Running:    out.Running,
		Reason:     out.Reason,
	}, nil
}

// PurgeResource 资源删除后清掉它的切片与向量。
func (cl *Client) PurgeResource(ctx context.Context, actor Actor, courseId, resourceId int64) error {
	if !cl.Enabled() {
		return ErrDisabled
	}
	if courseId <= 0 || resourceId <= 0 {
		return errors.New("缺少 courseId/resourceId")
	}
	callCtx, cancel := context.WithTimeout(ctx, purgeTimeout)
	defer cancel()
	body := map[string]interface{}{"course_id": courseId, "resource_id": resourceId}
	return cl.postJSON(callCtx, "/api/ai/kb/purge", actor, body, nil)
}

// TaskState 批改任务的进度视图（GET /api/ai/task/{taskId}）。
type TaskState struct {
	TaskId    string
	Status    string // pending / processing / done / failed / manual
	Error     string
	ItemCount int
	Result    *GradeResult // done / manual 时有值
}

// GradeResult 批改结果（一份提交的汇总 + 逐题明细）。
//
// 数字一律先按 float64 收：Python 那边 json 出来可能是 18 也可能是 18.0，
// 收成 int 会因为 "18.0 无法解析成 int" 直接报错。
type GradeResult struct {
	Score    int
	MaxScore int
	Comment  string
	ParseOk  bool
	Items    []GradeItemResult
}

// GradeItemResult 单题批改结果。
type GradeItemResult struct {
	QuestionId  int64
	Score       int
	MaxScore    int
	Comment     string
	ErrorPoints []string
	Suggestion  string
	ParseOk     bool
}

// GetGradeTask 查任务状态。任务不存在 / 已过期返回 ErrTaskGone。
func (cl *Client) GetGradeTask(ctx context.Context, actor Actor, taskId string) (*TaskState, error) {
	if !cl.Enabled() {
		return nil, ErrDisabled
	}
	if strings.TrimSpace(taskId) == "" {
		return nil, errors.New("缺少 taskId")
	}
	callCtx, cancel := context.WithTimeout(ctx, statusTimeout)
	defer cancel()

	var raw struct {
		TaskId    string          `json:"taskId"`
		Status    string          `json:"status"`
		Error     string          `json:"error"`
		ItemCount json.Number     `json:"itemCount"`
		Result    json.RawMessage `json:"result"`
	}
	path := "/api/ai/task/" + taskId
	if err := cl.getJSON(callCtx, path, actor, &raw); err != nil {
		if errors.Is(err, errNotFound) {
			return nil, ErrTaskGone
		}
		return nil, err
	}
	st := &TaskState{
		TaskId:    raw.TaskId,
		Status:    raw.Status,
		Error:     raw.Error,
		ItemCount: int(num(raw.ItemCount)),
	}
	if len(raw.Result) > 0 && string(raw.Result) != "null" {
		var g struct {
			Score    float64 `json:"score"`
			MaxScore float64 `json:"maxScore"`
			Comment  string  `json:"comment"`
			ParseOk  bool    `json:"parseOk"`
			Items    []struct {
				QuestionId  float64  `json:"questionId"`
				Score       *float64 `json:"score"`
				MaxScore    float64  `json:"maxScore"`
				Comment     string   `json:"comment"`
				ErrorPoints []string `json:"errorPoints"`
				Suggestion  string   `json:"suggestion"`
				ParseOk     bool     `json:"parseOk"`
			} `json:"items"`
		}
		if err := json.Unmarshal(raw.Result, &g); err != nil {
			return nil, fmt.Errorf("解析批改结果失败: %w", err)
		}
		res := &GradeResult{
			Score:    int(math.Round(g.Score)),
			MaxScore: int(math.Round(g.MaxScore)),
			Comment:  g.Comment,
			ParseOk:  g.ParseOk,
		}
		for _, it := range g.Items {
			one := GradeItemResult{
				QuestionId:  int64(math.Round(it.QuestionId)),
				MaxScore:    int(math.Round(it.MaxScore)),
				Comment:     it.Comment,
				ErrorPoints: it.ErrorPoints,
				Suggestion:  it.Suggestion,
				ParseOk:     it.ParseOk,
			}
			if it.Score != nil {
				one.Score = int(math.Round(*it.Score))
			}
			res.Items = append(res.Items, one)
		}
		st.Result = res
	}
	return st, nil
}

// ErrTaskGone：任务不存在或已过 Redis TTL。
var ErrTaskGone = errors.New("批改任务不存在或已过期")

var errNotFound = errors.New("not found")

func (cl *Client) postJSON(ctx context.Context, path string, actor Actor, body, out interface{}) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return cl.do(ctx, http.MethodPost, path, actor, buf, out)
}

func (cl *Client) getJSON(ctx context.Context, path string, actor Actor, out interface{}) error {
	return cl.do(ctx, http.MethodGet, path, actor, nil, out)
}

func (cl *Client) do(ctx context.Context, method, path string, actor Actor, body []byte, out interface{}) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, cl.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", cl.token)
	if actor.UserId > 0 {
		req.Header.Set("X-User-Id", strconv.FormatInt(actor.UserId, 10))
	}
	if actor.Role != "" {
		req.Header.Set("X-Role", actor.Role)
	}

	resp, err := cl.http.Do(req)
	if err != nil {
		// 网络层错误：ai-service 没起来 / 域名解析不了 / 超时
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w（ai-service %d）", ErrUnavailable, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		// 4xx 是 ai-service 明确拒绝（角色不对 / 参数不合法 / 内部令牌错），
		// 把它的 detail 原样带出去，老师能看到原因。
		return fmt.Errorf("AI 服务拒绝了请求（%d）：%s", resp.StatusCode, detailOf(payload))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return fmt.Errorf("解析 AI 服务响应失败: %w", err)
	}
	return nil
}

// detailOf 取 FastAPI 错误体的 detail；不是 JSON 或没 detail 就把原文截断返回。
func detailOf(payload []byte) string {
	var e struct {
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(payload, &e); err == nil && e.Detail != "" {
		return e.Detail
	}
	text := strings.TrimSpace(string(payload))
	if len(text) > 200 {
		return text[:200]
	}
	if text == "" {
		return "无响应内容"
	}
	return text
}

func num(n json.Number) float64 {
	f, err := n.Float64()
	if err != nil {
		return 0
	}
	return f
}
