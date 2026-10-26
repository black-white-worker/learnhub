package handler

import (
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/user"
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

	// 公开只读接口（首页课程列表、详情、视频播放、资源预览）
	publicRoutes := []rest.Route{
		{Method: http.MethodGet, Path: "/api/course/list", Handler: ListHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/detail/:id", Handler: CourseDetailHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/video/play/:courseId/:videoId", Handler: VideoPlayHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/resources/:courseId", Handler: ResourceListHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/resource/preview/:courseId/:resourceId", Handler: ResourcePreviewHandler(serverCtx)},
	}
	server.AddRoutes(publicRoutes)

	// 需登录的写操作接口
	protectedRoutes := []rest.Route{
		{Method: http.MethodPost, Path: "/api/course/create", Handler: CreateHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/cover", Handler: CoverUploadHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/videos", Handler: VideoUploadHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/resource", Handler: ResourceUploadHandler(serverCtx)},
		// 分片直传（视频大文件，前端直传 MinIO）
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/init", Handler: MultipartInitHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/sign", Handler: MultipartSignHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/complete", Handler: MultipartCompleteHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/upload/multipart/abort", Handler: MultipartAbortHandler(serverCtx)},
		{Method: http.MethodPut, Path: "/api/course/edit", Handler: EditCourseHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/course/delete/:id", Handler: DeleteCourseHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/course/resource/:courseId/:resourceId", Handler: ResourceDeleteHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/video/progress/:courseId/:videoId", Handler: SaveVideoProgressHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/video/progress/:courseId/:videoId", Handler: GetVideoProgressHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/course/note", Handler: addNoteHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/course/note/:courseId/:videoId", Handler: listNotesHandler(serverCtx)},
	}
	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, protectedRoutes...),
	)
}
