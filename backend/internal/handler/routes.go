package handler

import (
	classhandler "learningplatform/internal/app/class/handler"
	coursehandler "learningplatform/internal/app/course/handler"
	examhandler "learningplatform/internal/app/exam/handler"
	userhandler "learningplatform/internal/app/user/handler"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterHandlers 汇总所有业务模块的路由注册。
// 每新增一个业务模块（course、ai...），在这里加一行即可。
//
// 【冲突点 4】本文件由「课程列表」和「课程创建」两个分支各自修改：
// 两边都加了 course.RegisterRoutes(server, serverCtx) 这一行，
// 合并时只保留一行即可，别重复。
func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	userhandler.RegisterRoutes(server, serverCtx)
	coursehandler.RegisterRoutes(server, serverCtx)
	classhandler.RegisterRoutes(server, serverCtx)
	examhandler.RegisterRoutes(server, serverCtx)
	// 未来：ai.RegisterRoutes(server, serverCtx)
}
