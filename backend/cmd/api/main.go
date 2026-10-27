// 学习平台 API 服务入口。
//
// 目录约定（为后续拆微服务预留）：
//
//	cmd/api          当前单体入口；拆服务时每个服务一个 cmd/<name>
//	internal/app     按业务边界划分（user / course / class / exam），
//	                 每个包自带 handler + logic，可整体搬成独立服务
//	internal/domain  纯业务规则（authz / gradebook / examrules），零 IO 依赖
//	internal/platform 技术设施（oss / quota / middleware / response）
//	internal/model   MySQL 数据访问
//
// 拆微服务时：把 internal/app/<context> 连同它用到的 domain 包搬到新仓库，
// platform/model 按需复制或抽成共享库，HTTP 边界不用改。
package main

import (
	"flag"
	"fmt"

	"learningplatform/internal/app/course"
	examlogic "learningplatform/internal/app/exam/logic"
	"learningplatform/internal/config"
	"learningplatform/internal/handler"
	"learningplatform/internal/platform/middleware"
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

	// 全局中间件顺序即执行顺序：
	// 1. RequestID 先注入，后面的拒绝日志也能带上同一个 id；
	// 2. BodyLimit 在读 body 之前收紧上限（JSON 1MB，上传路由单独放宽）。
	server.Use(middleware.RequestIDMiddleware)
	server.Use(middleware.DefaultBodyLimitMiddleware().Handle)

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	handler.RegisterHealth(server, ctx)

	// 启动孤儿对象后台清理（每 10 分钟扫描一次 MinIO，
	// 删除"上传成功但课程未引用"的残留对象，网络波动兜底）
	course.StartOrphanSweeper(ctx)

	// 启动考试超时自动收卷（每分钟扫一次"考试中"的记录，
	// 超过"开考时间 + 时长"强制收卷，并按最后一次草稿判分）
	examlogic.StartAutoSubmitter(ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
