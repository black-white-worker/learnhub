package handler

// 健康检查端点。
//
// /healthz 只证明进程活着（容器 liveness），不碰任何外部依赖；
// /readyz  证明依赖可用（readiness），失败返回 503，让编排系统摘流量。
//
// 为什么单独拆开：把 MySQL/Redis 抖动也塞进 liveness，会导致容器被反复重启，
// 而重启并不能修复数据库。这是 Kubernetes / docker healthcheck 的标准区分。

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterHealth 注册 /healthz 与 /readyz（无需登录）。
func RegisterHealth(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/healthz", Handler: livenessHandler()},
		{Method: http.MethodGet, Path: "/readyz", Handler: readinessHandler(svcCtx)},
	})
}

func livenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeHealth(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func readinessHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		checks := map[string]string{}
		status := http.StatusOK

		if svcCtx.DB == nil {
			checks["mysql"] = "not configured"
		} else {
			var one int
			if err := svcCtx.DB.QueryRowCtx(ctx, &one, "SELECT 1"); err != nil {
				checks["mysql"] = "down: " + err.Error()
				status = http.StatusServiceUnavailable
			} else {
				checks["mysql"] = "ok"
			}
		}

		if svcCtx.Redis == nil {
			checks["redis"] = "not configured"
		} else if !svcCtx.Redis.Ping() {
			checks["redis"] = "down"
			status = http.StatusServiceUnavailable
		} else {
			checks["redis"] = "ok"
		}

		// 对象存储没有 Ping 语义，只报告是否已初始化；
		// 真正要用时失败会在各自接口上暴露。
		if svcCtx.Oss == nil {
			checks["minio"] = "not configured"
		} else {
			checks["minio"] = "configured"
		}

		if status != http.StatusOK {
			checks["status"] = "degraded"
		} else {
			checks["status"] = "ok"
		}
		writeHealth(w, status, checks)
	}
}

func writeHealth(w http.ResponseWriter, status int, body map[string]string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
