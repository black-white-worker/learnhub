package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/user"
	"learningplatform/internal/platform/middleware"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRoutes 注册课程模块路由。
// 问题1：首页课程展示不需要登录就能看见——只读 GET 接口公开，
// 写操作（创建/上传/编辑/删除/笔记/进度）仍需登录。
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	if serverCtx.CourseStore != nil {
		course.InitCourseStore(serverCtx.CourseStore)
	}

	// 公开只读接口（首页课程列表、课程详情）。
	// 用"可选登录"中间件：带了 token 就识别用户（详情据此返回 canWatch / 视频地址），
	// 没带 token 也照常放行，按匿名展示。
	publicRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/api/course/list", Handler: ListHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/detail/:id", Handler: CourseDetailHandler(serverCtx)},
	}
	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).HandleOptional, publicRoutes...),
	)

	// 需登录的写操作 + 媒体读取接口。
	// 媒体（播放 / 资源列表 / 在线预览）返回的是预签名 URL，
	// 必须绑定登录态与选课关系，不能放在公开路由上。
	protectedRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/api/course/video/play/:courseId/:videoId", Handler: VideoPlayHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/resources/:courseId", Handler: ResourceListHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/resource/preview/:courseId/:resourceId", Handler: ResourcePreviewHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/create", Handler: CreateHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/cover", Handler: CoverUploadHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/videos", Handler: VideoUploadHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/resource", Handler: ResourceUploadHandler(serverCtx)},
		// 分片直传（视频大文件，前端直传 MinIO）
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/init", Handler: MultipartInitHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/sign", Handler: MultipartSignHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/complete", Handler: MultipartCompleteHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/abort", Handler: MultipartAbortHandler(serverCtx)},
		// 断点续传：刷新页面后按 uploadId 查已完成分片，跳过重传
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/status", Handler: MultipartStatusHandler(serverCtx)},
		// 视频后处理（faststart）失败时人工重新排队
		{Method: http.MethodPost, Path: "/api/course/video/reprocess/:courseId/:videoId", Handler: ReprocessVideoHandler(serverCtx)},
		{Method: http.MethodPut, Path: "/api/course/edit", Handler: EditCourseHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/course/delete/:id", Handler: DeleteCourseHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/course/resource/:courseId/:resourceId", Handler: ResourceDeleteHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/video/progress/:courseId/:videoId", Handler: SaveVideoProgressHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/video/progress/:courseId/:videoId", Handler: GetVideoProgressHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/note", Handler: addNoteHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/note/:courseId/:videoId", Handler: listNotesHandler(serverCtx)},
		// 整门课的笔记（路径用 notes 复数，避免和上面的 :courseId/:videoId 撞路由）
		{Method: http.MethodGet, Path: "/api/course/notes/summary/:courseId", Handler: summaryNotesHandler(serverCtx)},
	}
	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, protectedRoutes...),
	)

	// ai-service 取数入口：灌库与答疑共用同一份「课程资料」清单（讲义 + 视频）。
	// 它返回整门课的下载地址，权限比「登录即可读」更高，所以只认 X-Internal-Token，
	// 且不挂登录中间件 —— 服务间调用手里没有用户 JWT，也不该去伪造一个。
	server.AddRoutes(
		rest.WithMiddleware(
			middleware.NewInternalTokenMiddleware(serverCtx.Config.AiService.InternalToken).Handle,
			rest.Route{
				Method:  http.MethodGet,
				Path:    "/api/internal/course/:courseId/kb-manifest",
				Handler: KbManifestHandler(serverCtx),
			},
		),
	)
}
