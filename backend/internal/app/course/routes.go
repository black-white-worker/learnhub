package course

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册课程模块的所有路由。
//
// 【冲突点 2】本文件由「课程列表」和「课程创建」两个分支各自编写，
// 合并到「课程管理功能」主干时整个文件冲突，需要手动合并两边的路由条目。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			// 课程创建接口（模拟"同事李四"开发的功能）
			{
				Method:  http.MethodPost,
				Path:    "/api/course/create",
				Handler: CreateHandler(serverCtx),
			},
		},
	)
}
