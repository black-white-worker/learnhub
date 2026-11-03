package handler

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// NewAIProxy 创建反向代理，把 /api/ai/* 转发到 ai-service:8000。
//
// 前端只访问 Go 主服务（8888），Go 验完 JWT 后把请求透传给 Python AI 服务。
// X-User-Id 头由 Go 从 JWT 解出来后注入，AI 服务信任这个头。
func NewAIProxy(aiBackend string) http.HandlerFunc {
	target, _ := url.Parse(aiBackend)
	proxy := httputil.NewSingleHostReverseProxy(target)

	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
	}

	return func(w http.ResponseWriter, r *http.Request) {
		// SSE 需要关闭 nginx/反向代理缓冲
		w.Header().Set("X-Accel-Buffering", "no")
		proxy.ServeHTTP(w, r)
	}
}

// AIProxyPaths 需要代理到 ai-service 的所有路径。
// go-zero 路由不支持通配前缀，这里显式列出。
var AIProxyPaths = []string{
	"/api/ai/chat",
	"/api/ai/summary",
	"/api/ai/generate",
	"/api/ai/session/history",
	"/api/ai/session",
}

// IsAIPath 判断是否是 AI 相关路径（中间件用）
func IsAIPath(path string) bool {
	for _, p := range AIProxyPaths {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
