package course

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册课程模块的所有路由。
// 本次合并：保留「课程创建」分支的 create + 封面上传 + 视频上传，
// 以及「课程列表」分支的 list，共 4 条路由。
// 注意：810fb6d 合并时本文件被误提交为空文件（编辑器操作导致），
// 由后续修复提交恢复为 4 条路由版本。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	server.AddRoutes(
		[]rest.Route{
			// 课程创建接口（来自「课程创建」分支）
			{
				Method:  http.MethodPost,
				Path:    "/api/course/create",
				Handler: CreateHandler(serverCtx),
			},
			// 课程封面上传接口（multipart：courseId + file，生成原图+缩略图存 MinIO）
			{
				Method:  http.MethodPost,
				Path:    "/api/course/upload/cover",
				Handler: CoverUploadHandler(serverCtx),
			},
			// 课程视频上传接口（multipart：courseId + files，一次可传多个视频）
			{
				Method:  http.MethodPost,
				Path:    "/api/course/upload/videos",
				Handler: VideoUploadHandler(serverCtx),
			},
			// 课程列表接口（来自「课程列表」分支）
			{
				Method:  http.MethodGet,
				Path:    "/api/course/list",
				Handler: ListHandler(serverCtx),
			},
		},
	)
}
