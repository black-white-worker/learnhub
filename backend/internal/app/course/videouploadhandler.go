package course

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// VideoUploadHandler 视频上传接口的 HTTP 处理器：
// 解析 multipart 表单 → 逐个取出视频文件流 → 调用业务逻辑 → 返回 JSON。
//
// 前端调用示例（FormData）：
//
//	formData.append("courseId", "1")
//	formData.append("files", videoFile1)
//	formData.append("files", videoFile2)
//	POST /api/course/upload/videos
func VideoUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析 multipart 表单（32MB 以内先放内存，超出落临时文件）
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer r.MultipartForm.RemoveAll() // 用完清理临时文件

		// 2. 课程 id（表单字段 courseId）
		courseId, err := strconv.Atoi(r.FormValue("courseId"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errors.New("courseId 必须是数字"))
			return
		}

		// 3. 视频文件（表单字段 files，可多个）
		fileHeaders := r.MultipartForm.File["files"]
		if len(fileHeaders) == 0 {
			httpx.ErrorCtx(r.Context(), w, errors.New("请至少上传一个视频文件（字段名 files）"))
			return
		}

		// 4. 逐个打开文件流。
		// 大文件在内存里只保留文件头信息，内容通过 Reader 流式读取，避免撑爆内存。
		req := &UploadVideosRequest{CourseId: courseId}
		for _, fh := range fileHeaders {
			f, err := fh.Open()
			if err != nil {
				closeUploadFiles(req)
				httpx.ErrorCtx(r.Context(), w, err)
				return
			}
			req.Files = append(req.Files, &UploadFile{
				FileName: fh.Filename,
				Size:     fh.Size,
				Reader:   f,
			})
		}

		// 5. 调用视频上传业务逻辑
		l := NewVideoUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadVideos(req)

		// 6. 无论成败，关闭所有文件流，释放临时文件句柄
		closeUploadFiles(req)

		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// closeUploadFiles 关闭请求中所有已打开的文件流，释放临时文件句柄。
func closeUploadFiles(req *UploadVideosRequest) {
	for _, f := range req.Files {
		if c, ok := f.Reader.(io.Closer); ok {
			c.Close()
		}
	}
}
