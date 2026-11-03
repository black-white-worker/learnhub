// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package main

import (
	"flag"
	"fmt"
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/config"
	"learningplatform/internal/handler"
	"learningplatform/internal/middleware"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/learningplatform-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	// conf.UseEnv() 允许配置文件里的 ${环境变量} 被替换，Docker 部署时注入连接信息
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// 全局中间件：注入 X-Request-ID，串起一次请求的所有日志
	server.Use(middleware.RequestIDMiddleware)

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// AI 服务反向代理：/api/ai/* 转发到 Python FastAPI 服务
	// docker 内网用服务名 ai-service:8000；本地开发用 127.0.0.1:8000
	aiBackend := "http://ai-service:8000"
	if c.Mode == "dev" {
		aiBackend = "http://127.0.0.1:8000"
	}
	aiProxy := handler.NewAIProxy(aiBackend)
	for _, p := range handler.AIProxyPaths {
		server.AddRoute(rest.Route{
			Method:  http.MethodPost,
			Path:    p,
			Handler: aiProxy,
		})
		server.AddRoute(rest.Route{
			Method:  http.MethodGet,
			Path:    p,
			Handler: aiProxy,
		})
		server.AddRoute(rest.Route{
			Method:  http.MethodDelete,
			Path:    p,
			Handler: aiProxy,
		})
	}

	// 启动孤儿对象后台清理（每 10 分钟扫描一次 MinIO，
	// 删除"上传成功但课程未引用"的残留封面/视频，网络波动兜底）
	course.StartOrphanSweeper(ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
