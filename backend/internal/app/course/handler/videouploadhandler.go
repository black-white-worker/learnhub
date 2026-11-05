package handler

import (
	"io"
	"net/http"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/domain/authz"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// VideoUploadHandler 视频上传：POST /api/course/upload/videos?courseId=1
//
// 前端 FormData 里只需要放文件（字段名 files，可多个），课程 id 走 query ——
// 这样"是不是这门课的主人"可以在解析 1GB body 之前判掉。
func VideoUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 课程 id + 鉴权（在读取 body 之前）
		courseId, ok := courseIdFromQuery(w, r)
		if !ok {
			return
		}
		if !authorizeCourse(w, r, courseId, authz.CourseUpload) {
			return
		}

		// 2. 解析 multipart 表单（32MB 以内先放内存，超出落临时文件）
		if !parseMultipartForm(w, r, 32<<20) {
			return
		}
		defer r.MultipartForm.RemoveAll() // 用完清理临时文件

		// 3. 视频文件（表单字段 files，可多个）
		fileHeaders := r.MultipartForm.File["files"]
		if len(fileHeaders) == 0 {
			courseError(r, w, course.ErrBadRequest("请至少上传一个视频文件（字段名 files）"))
			return
		}

		// 4. 每日上传配额：按本次所有视频的总字节数预占（未配置则不限制）
		var totalBytes int64
		for _, fh := range fileHeaders {
			totalBytes += fh.Size
		}
		release, ok := reserveUploadQuota(w, r, svcCtx, totalBytes)
		if !ok {
			return
		}
		succeeded := false
		defer func() {
			if !succeeded {
				release()
			}
		}()

		// 5. 逐个打开文件流。
		// 大文件在内存里只保留文件头信息，内容通过 Reader 流式读取，避免撑爆内存。
		req := &course.UploadVideosRequest{CourseId: courseId}
		for _, fh := range fileHeaders {
			f, err := fh.Open()
			if err != nil {
				closeUploadFiles(req)
				courseError(r, w, err)
				return
			}
			req.Files = append(req.Files, &course.UploadFile{
				FileName: fh.Filename,
				Size:     fh.Size,
				Reader:   f,
			})
		}

		// 6. 调用视频上传业务逻辑
		l := logic.NewVideoUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadVideos(req)

		// 7. 无论成败，关闭所有文件流，释放临时文件句柄
		closeUploadFiles(req)

		if err != nil {
			courseError(r, w, err)
			return
		}
		succeeded = true
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}

// closeUploadFiles 关闭请求中所有已打开的文件流，释放临时文件句柄。
func closeUploadFiles(req *course.UploadVideosRequest) {
	for _, f := range req.Files {
		if c, ok := f.Reader.(io.Closer); ok {
			c.Close()
		}
	}
}
