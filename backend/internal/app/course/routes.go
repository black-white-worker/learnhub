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
	// 生产环境：把课程存储从内存实现切换为 MySQL 实现（持久化）。
	// 测试环境不会走到这里（newTestSvcCtx 不带 CourseStore，store 仍是内存版）。
	if serverCtx.CourseStore != nil {
		InitCourseStore(serverCtx.CourseStore)
	}

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
			// ===== 以下为「课程观看」分支新增 =====
			// 获取视频播放信息：GET /api/course/video/play/:courseId/:videoId
			{
				Method:  http.MethodGet,
				Path:    "/api/course/video/play/:courseId/:videoId",
				Handler: VideoPlayHandler(serverCtx),
			},
			// 上报播放进度：POST /api/course/video/progress/:courseId/:videoId
			{
				Method:  http.MethodPost,
				Path:    "/api/course/video/progress/:courseId/:videoId",
				Handler: SaveVideoProgressHandler(serverCtx),
			},
			// 读取播放进度：GET /api/course/video/progress/:courseId/:videoId
			{
				Method:  http.MethodGet,
				Path:    "/api/course/video/progress/:courseId/:videoId",
				Handler: GetVideoProgressHandler(serverCtx),
			},
			// ===== 以下为「课程编辑」分支新增 =====
			// 课程详情接口：GET /api/course/detail/:id（卡片点击进入详情页）
			{
				Method:  http.MethodGet,
				Path:    "/api/course/detail/:id",
				Handler: CourseDetailHandler(serverCtx),
			},
			// 编辑课程基本信息：PUT /api/course/edit（name / intro / teacher）
			{
				Method:  http.MethodPut,
				Path:    "/api/course/edit",
				Handler: EditCourseHandler(serverCtx),
			},
			// 删除课程：DELETE /api/course/delete/:id
			{
				Method:  http.MethodDelete,
				Path:    "/api/course/delete/:id",
				Handler: DeleteCourseHandler(serverCtx),
			},
			// ===== 以下为「课程资源」分支新增 =====
			// 上传文档资源（PPT / Word / PDF）：POST /api/course/upload/resource
			{
				Method:  http.MethodPost,
				Path:    "/api/course/upload/resource",
				Handler: ResourceUploadHandler(serverCtx),
			},
			// 课程资源列表：GET /api/course/resources/:courseId
			{
				Method:  http.MethodGet,
				Path:    "/api/course/resources/:courseId",
				Handler: ResourceListHandler(serverCtx),
			},
			// 删除课程资源：DELETE /api/course/resource/:courseId/:resourceId
			{
				Method:  http.MethodDelete,
				Path:    "/api/course/resource/:courseId/:resourceId",
				Handler: ResourceDeleteHandler(serverCtx),
			},
			// 资源在线预览：GET /api/course/resource/preview/:courseId/:resourceId
			// PDF 直跳 MinIO，docx/pptx 后端用 LibreOffice 转 PDF 后流式返回
			{
				Method:  http.MethodGet,
				Path:    "/api/course/resource/preview/:courseId/:resourceId",
				Handler: ResourcePreviewHandler(serverCtx),
			},
		},
	)
}
