package course

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册课程模块的所有路由。
//
// 【冲突点 2】本文件由「课程列表」和「课程创建」两个分支各自编写：
// 「课程创建」分支注册了 POST /api/course/create，
// 本分支注册了 GET /api/course/list。
// 合并时把两边的路由条目都保留。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			// 课程列表接口（模拟"员工 B"开发的功能）
			{
				Method:  http.MethodGet,
				Path:    "/api/course/list",
				Handler: ListHandler(serverCtx),
			},
		},
	)
}
