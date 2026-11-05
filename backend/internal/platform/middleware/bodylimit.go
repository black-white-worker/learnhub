// Package middleware 全局 HTTP 中间件。
package middleware

// 请求体大小限制。
//
// 背景：rest 的 MaxBytes 配成 1GB（为了视频上传），但它是全局的 ——
// 任何 JSON 接口都会接受 1GB 的请求体，而 handler 里的 json.Decoder
// 又没有第二次限长，一个登录用户就能用大 body 把进程内存打满。
//
// 做法：按路径前缀给不同上限，默认 1MB。上传路由显式放宽，
// 其余接口（包括新增的业务接口）自动受到保护，不需要每个 handler 自己记得限长。
//
// 与开源实现的对应关系：这就是 go-zero / gin 里常见的 BodyLimit 中间件，
// 统一包一层 http.MaxBytesReader，超出时返回 413 而不是 500。

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"learningplatform/internal/platform/response"
)

// 默认限制：普通 JSON 接口 1MB，足够覆盖出题、交卷等最大载荷。
const (
	DefaultBodyLimit = 1 << 20 // 1MB
	// UploadBodyLimit 大文件上传上限（视频 500MB × 多个 + multipart 开销）。
	UploadBodyLimit = 1 << 30 // 1GB
)

// BodyLimitRule 路径前缀 → 上限。
type BodyLimitRule struct {
	// PathPrefix 请求路径前缀，例如 "/api/course/upload/"。
	PathPrefix string
	// Limit 允许的最大字节数。
	Limit int64
}

// BodyLimit 请求体限流中间件。
//
// 零值可用：Default <= 0 时使用 DefaultBodyLimit。
type BodyLimit struct {
	Default   int64
	Overrides []BodyLimitRule
}

// DefaultBodyLimitMiddleware 生产环境使用的标准配置。
func DefaultBodyLimitMiddleware() BodyLimit {
	return BodyLimit{
		Default: DefaultBodyLimit,
		Overrides: []BodyLimitRule{
			// 视频/封面/讲义上传：handler 内部还会按文件类型二次校验
			{PathPrefix: "/api/course/upload/", Limit: UploadBodyLimit},
			// 作业附件：50MB 上限 + multipart 开销
			{PathPrefix: "/api/assignment/upload", Limit: 64 << 20},
		},
	}
}

// limitFor 返回该路径生效的上限。多个规则命中时取最大值 ——
// 前缀更具体的规则通常是放宽上传，不应被更宽的默认值盖掉。
func (b BodyLimit) limitFor(path string) int64 {
	limit := b.Default
	if limit <= 0 {
		limit = DefaultBodyLimit
	}
	for _, r := range b.Overrides {
		if strings.HasPrefix(path, r.PathPrefix) && r.Limit > limit {
			limit = r.Limit
		}
	}
	return limit
}

// Handle 实现 go-zero 的 middleware 签名。
func (b BodyLimit) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := b.limitFor(r.URL.Path)

		// Content-Length 已知时先拒绝，省掉一次读盘/读内存。
		if r.ContentLength > limit {
			writeTooLarge(w, limit)
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next(w, r)
	}
}

// TooLargeError 请求体超限错误。实现 StatusCode() 让 response 包输出 413。
type TooLargeError struct {
	Limit int64
}

func (e *TooLargeError) Error() string {
	return fmt.Sprintf("请求体过大（上限 %d MB）", e.Limit>>20)
}

// StatusCode 供 response.codedError 识别。
func (e *TooLargeError) StatusCode() int { return http.StatusRequestEntityTooLarge }

// BodyTooLarge 判断错误是否来自 MaxBytesReader。
//
// handler 在 json 解码 / ParseMultipartForm 失败时调用它，
// 把"body 太大"翻译成 413，而不是笼统的 500 或"参数错误"。
func BodyTooLarge(err error) bool {
	if err == nil {
		return false
	}
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return true
	}
	// MaxBytesReader 在部分路径下只返回字符串错误
	return strings.Contains(err.Error(), "http: request body too large")
}

// writeTooLarge 直接输出 413 统一响应。
func writeTooLarge(w http.ResponseWriter, limit int64) {
	response.Error(w, &TooLargeError{Limit: limit})
}
