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

// ResourceUploadHandler 讲义上传：POST /api/course/upload/resource?courseId=1
//
// 先鉴权、再解析 body：未授权请求不会占用临时目录和上传配额。
func ResourceUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		courseId, ok := courseIdFromQuery(w, r)
		if !ok {
			return
		}
		if !authorizeCourse(w, r, courseId, authz.CourseUpload) {
			return
		}

		if !parseMultipartForm(w, r, 64<<20) {
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			courseError(r, w, course.ErrBadRequest("请上传资源文件（字段名 file）"))
			return
		}
		defer file.Close()

		maxBytes := course.EffectiveMaxResourceBytes(svcCtx)
		if header.Size > maxBytes {
			courseError(r, w, &course.BizError{Code: http.StatusRequestEntityTooLarge, Msg: "资源文件超过大小上限"})
			return
		}

		// 每日上传配额（与封面/视频共用同一份额度），失败要归还
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

		l := logic.NewResourceUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadResource(&course.UploadResourceRequest{
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
