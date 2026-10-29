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

// CoverUploadHandler 封面上传：POST /api/course/upload/cover?courseId=1
//
// 处理顺序（重要）：先鉴权 → 再解析 multipart → 再读文件字节。
// 这样未授权的请求不会先把文件落盘，也不会占满上传配额。
func CoverUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 课程 id（query 参数，见 courseIdFromQuery 的说明）
		courseId, ok := courseIdFromQuery(w, r)
		if !ok {
			return
		}
		// 2. 鉴权：只有课程创建者/管理员能传封面
		if !authorizeCourse(w, r, courseId, authz.CourseUpload) {
			return
		}

		// 3. 解析 multipart（32MB 以内放内存，超出落临时文件）
		if !parseMultipartForm(w, r, 32<<20) {
			return
		}
		defer r.MultipartForm.RemoveAll() // 用完清理临时文件

		file, header, err := r.FormFile("file")
		if err != nil {
			courseError(r, w, course.ErrBadRequest("请上传封面文件（字段名 file）"))
			return
		}
		defer file.Close()

		// 4. 大小预检（逻辑层还会按配置再校验一次）
		maxBytes := course.EffectiveMaxCoverBytes(svcCtx)
		if header.Size > maxBytes {
			courseError(r, w, &course.BizError{Code: http.StatusRequestEntityTooLarge, Msg: "封面文件超过大小上限"})
			return
		}

		// 5. 预占配额；后续任一步失败都要归还，成功则保留
		release, ok := reserveUploadQuota(w, r, svcCtx, header.Size)
		if !ok {
			return
		}
		succeeded := false
		defer func() {
			if !succeeded {
				release()
			}
		}()

		data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			courseError(r, w, err)
			return
		}

		l := logic.NewCoverUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadCover(&course.UploadCoverRequest{
			CourseId:  courseId,
			FileName:  header.Filename,
			FileBytes: data,
		})
		if err != nil {
			courseError(r, w, err)
			return
		}
		succeeded = true
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
