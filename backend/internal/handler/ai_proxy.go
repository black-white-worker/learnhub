package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"
)

// 内部调用令牌：Go 代理 → Python AI 服务之间的共享密钥。
// Python 侧校验这个头，防止绕过 Go 直连 8000 端口。
const internalToken = "ai-internal-secret-2026"

// NewAIProxy 创建反向代理，把 /api/ai/* 转发到 ai-service:8000。
//
// 流程：
//  1. 从 JWT 解析当前用户（user.CurrentUser）
//  2. strip 客户端传入的 X-User-Id / X-Role（防伪造）
//  3. 写入可信的 X-User-Id、X-Role、X-Internal-Token
//  4. 透传到 Python FastAPI
func NewAIProxy(aiBackend string, svcCtx *svc.ServiceContext) http.HandlerFunc {
	target, _ := url.Parse(aiBackend)
	proxy := httputil.NewSingleHostReverseProxy(target)

	// SSE 是长连接：只卡响应头超时，不卡整体请求时长
	proxy.Transport = &http.Transport{
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       120 * time.Second,
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
		if u != nil && u.Username != "" {
			uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
			if err == nil && uid > 0 {
				req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
			}
			req.Header.Set("X-Role", u.Role)
		}

		// 内部调用令牌：Python 侧校验
		req.Header.Set("X-Internal-Token", internalToken)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE 关闭缓冲
		w.Header().Set("X-Accel-Buffering", "no")
		proxy.ServeHTTP(w, r)
	}
}

// AIProxyPaths 需要代理到 ai-service 的所有路径。
var AIProxyPaths = []string{
	"/api/ai/chat",
	"/api/ai/summary",
	"/api/ai/generate",
	"/api/ai/session/history",
	"/api/ai/session",
}

// IsAIPath 判断是否是 AI 相关路径
func IsAIPath(path string) bool {
	for _, p := range AIProxyPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
