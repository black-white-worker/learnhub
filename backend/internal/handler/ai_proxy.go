package handler

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// aiRoute 一条需要转发到 ai-service 的路由。
// 方法 + 路径都必须显式列出：go-zero 的路由不支持通配符，
// 而"只放行白名单"正是我们要的 —— 没列出来的 Python 接口外部一律 404。
type aiRoute struct {
	Method string
	Path   string
}

// aiRoutes 是 Go 对外暴露的 AI 接口清单，也是 ai-service 侧的路径。
// 加接口 = 这里加一行 + Python 侧加一个 router。
var aiRoutes = []aiRoute{
	{http.MethodPost, "/api/ai/chat"},           // 答疑（SSE 流式）
	{http.MethodPost, "/api/ai/generate"},       // AI 出题
	{http.MethodGet, "/api/ai/session/history"}, // 对话历史
	{http.MethodDelete, "/api/ai/session"},      // 清空对话历史
	{http.MethodGet, "/api/ai/kb/status"},       // 知识库覆盖度（只返回聚合数字，不含提问原文）
}

// AI 代理的超时分层（自外向内逐级放宽，保证超时先由前端给友好提示）：
//
//	nginx proxy_read_timeout 300s > go-zero 路由 180s > 本代理非 SSE 165s
//	> 前端 axios 150s > ai-service 的 LLM 调用与最多两轮重试
//
// 关键区别：SSE 的响应头是「立刻」发出的（之后才是持续几十秒的流），
// 所以可以只卡响应头 10s；而普通 JSON 接口（出题）要等外网检索中文资料
// + 等大模型生成完才写第一个响应头，实测 50~120s。给它们设 10s 响应头超时，
// 会让每次中文专业出题都在第 10 秒被掐断、由 ErrorHandler 返回 503。
const (
	sseHeaderTimeout  = 10 * time.Second  // SSE：只等响应头，流本身不受限
	aiJSONTimeout     = 165 * time.Second // 非 SSE：整体请求上限
	aiRouteTimeout    = 180 * time.Second // go-zero 路由层上限
	aiIdleConnTimeout = 120 * time.Second
)

// 注意：批改入口 POST /api/ai/grade 与任务状态 GET /api/ai/task/{id} 不在这张表里。
// 批改要烧 LLM 额度、还要往成绩册写"AI 初评"，所以载荷必须由 Go 从数据库组装
// （见 platform/aiservice.Client），不能让浏览器自己拼一份 standardAnswer 上来。

// RegisterAIRoutes 把 AI 接口挂到 HTTP 服务上。
//
// 两层门槛：
//  1. 走登录鉴权中间件 —— 代理要靠 JWT 才能拿到可信的用户身份；
//  2. 代理内部再注入 X-User-Id / X-Role / X-Internal-Token。
//
// ai-service 未配置（Url 或 InternalToken 为空）时仍然注册路由，
// 但直接回 503：前端能拿到明确文案，而不是 nginx 的 404 页面。
func RegisterAIRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	proxy := NewAIProxy(serverCtx)
	routes := make([]rest.Route, 0, len(aiRoutes)+1)
	// 状态查询由 Go 自己回答（不转发）：前端要用它决定 AI 菜单和按钮是否可用。
	// 配置没齐时 python 根本没启动，转发过去只会是连不上，问 Go 才有意义。
	routes = append(routes, rest.Route{Method: http.MethodGet, Path: "/api/ai/status", Handler: aiStatusHandler(serverCtx)})
	for _, r := range aiRoutes {
		routes = append(routes, rest.Route{Method: r.Method, Path: r.Path, Handler: proxy})
	}
	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, routes...),
		// AI 接口要等「中文资料检索 + 大模型生成 + 可能的换蓝图重试」，
		// 实测一次出题 50~120s；go-zero 全局默认只有 3s，必须单独放宽。
		// 放宽到 180s（大于前端 axios 的 150s，保证真超时时先由前端给友好提示）。
		// SSE 答疑（Accept: text/event-stream）会被 go-zero 的 TimeoutHandler
		// 直接跳过，不受此设置影响，流式输出仍然逐块返回。
		// 注意：路由超时之外，代理自身的 http.Transport 也有独立的响应头超时，
		// 见 NewAIProxy —— 那一层才是这次 503 的真正来源。
		rest.WithTimeout(aiRouteTimeout),
	)
}

// aiStatusHandler GET /api/ai/status
// enabled=false 时 reason 给出人话原因，前端把菜单置灰而不是点了报错。
func aiStatusHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enabled := svcCtx.AI.Enabled()
		reason := ""
		if !enabled {
			reason = "AI 功能未启用，请联系管理员配置 AiService"
		}
		httpx.WriteJson(w, http.StatusOK, map[string]interface{}{
			"enabled": enabled,
			"reason":  reason,
			// 前端按这个清单挂菜单：加功能只改 Go 一处，不用发前端
			"features": []string{"chat", "generate", "grade"},
		})
	}
}

// NewAIProxy 创建反向代理，把 /api/ai/* 转发到 ai-service。
//
// 流程：
//  1. 从 JWT 解析当前用户（user.CurrentUser）
//  2. strip 客户端传入的 X-User-Id / X-Role（防伪造）
//  3. 写入可信的 X-User-Id、X-Role、X-Internal-Token
//  4. 透传到 Python FastAPI
func NewAIProxy(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return newAIProxy(svcCtx, sseHeaderTimeout, aiJSONTimeout)
}

// newAIProxy 把超时做成参数：单测可以用几十毫秒复现「上游慢导致 503」，
// 不用真的等 10 秒。生产参数由 NewAIProxy 传入。
func newAIProxy(svcCtx *svc.ServiceContext, sseHeader, jsonTimeout time.Duration) http.HandlerFunc {
	ai := svcCtx.Config.AiService

	target, err := url.Parse(strings.TrimSpace(ai.Url))
	if err != nil || target.Host == "" || ai.InternalToken == "" {
		logx.Errorf("AI 服务未正确配置（AiService.Url=%q, InternalToken 是否为空=%v），/api/ai/* 将返回 503",
			ai.Url, ai.InternalToken == "")
		return aiUnavailable
	}
	if target.Scheme == "" {
		target.Scheme = "http"
	}

	// build 造一条共用 Director / ErrorHandler 的转发链，只有响应头超时不同。
	//   - SSE：上游立刻给响应头，10s 内没给就说明挂了，快速失败；
	//   - 普通 JSON：上游处理完才给响应头（出题实测 50~120s），必须传 0
	//     （= 不限制），整体时长交给外层 context 控制。
	build := func(headerTimeout time.Duration) *httputil.ReverseProxy {
		proxy := httputil.NewSingleHostReverseProxy(target)
		proxy.Transport = &http.Transport{
			ResponseHeaderTimeout: headerTimeout,
			IdleConnTimeout:       aiIdleConnTimeout,
		}

		// 后端不可达时默认会写一个裸的 502 HTML，前端拿不到 JSON 信封；
		// 换成统一的 {code,message}，和平台其他接口一致。
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			logx.Errorf("转发 ai-service 失败 %s %s: %v", r.Method, r.URL.Path, e)
			response.Error(w, &aiError{msg: "AI 服务暂时不可用，请稍后重试"})
		}

		director := proxy.Director
		proxy.Director = func(req *http.Request) {
			director(req)
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host

			// 关键安全：strip 客户端伪造的身份头，用 Go 解出来的可信值覆盖
			req.Header.Del("X-User-Id")
			req.Header.Del("X-Role")

			// 从 JWT 解出的用户信息注入
			u := user.CurrentUser(req)
			if u != nil {
				uid := u.Id
				if uid <= 0 {
					// 老会话可能没落 Id（SessionUser.Id 是后加的），按用户名回查一次
					id, err := svcCtx.UserStore.GetIdByUsername(u.Username)
					if err == nil {
						uid = id
					}
				}
				if uid > 0 {
					req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
				}
				req.Header.Set("X-Role", u.Role)
			}

			// 内部调用令牌：Python 侧校验
			req.Header.Set("X-Internal-Token", ai.InternalToken)
		}
		return proxy
	}

	sseProxy := build(sseHeader)
	jsonProxy := build(0)

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE 关闭缓冲（nginx 见到这个头会逐块转发，而不是等整个响应攒完）
		w.Header().Set("X-Accel-Buffering", "no")
		if isAIStream(r) {
			sseProxy.ServeHTTP(w, r)
			return
		}
		// 普通 JSON 接口：不卡响应头，只卡整体时长，避免长命题被提前掐断。
		ctx, cancel := context.WithTimeout(r.Context(), jsonTimeout)
		defer cancel()
		jsonProxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// isAIStream 与 go-zero TimeoutHandler 的判定口径保持一致：
// 前端的流式答疑固定带 Accept: text/event-stream。
func isAIStream(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

// aiUnavailable：AI 未配置时的兜底 handler。503 + 统一信封。
func aiUnavailable(w http.ResponseWriter, r *http.Request) {
	response.Error(w, &aiError{msg: "AI 功能未启用，请联系管理员配置 AiService"})
}

// aiError 带 StatusCode()，让 response.Error 输出 HTTP 503 + code=503。
type aiError struct{ msg string }

func (e *aiError) Error() string { return e.msg }
func (e *aiError) StatusCode() int {
	return http.StatusServiceUnavailable
}
