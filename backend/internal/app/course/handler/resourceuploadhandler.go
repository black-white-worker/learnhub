package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/authz"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ResourceUploadHandler 课程资源上传接口的 HTTP 处理器：
// 解析 multipart 表单（courseId + file）→ 读出字节 → 调用业务逻辑 → 返回 JSON。
//
// 前端调用示例（FormData）：
//
//	formData.append("courseId", "1")
//	formData.append("file", pptFile)
//	POST /api/course/upload/resource
func ResourceUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		defer r.MultipartForm.RemoveAll()

		courseId, err := strconv.Atoi(r.FormValue("courseId"))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errors.New("courseId 必须是数字"))
			return
		}
		// 只有课程创建者/管理员能往这门课上传讲义
		if !authorizeCourse(w, r, courseId, authz.CourseUpload) {
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errors.New("请上传资源文件（字段名 file）"))
			return
		}
		defer file.Close()

		maxBytes := course.EffectiveMaxResourceBytes(svcCtx)
		if header.Size > maxBytes {
			httpx.ErrorCtx(r.Context(), w, errors.New("资源文件超过大小上限"))
			return
		}
		// 每日上传配额（与封面/视频共用同一份额度）
		if !uploadQuotaGuard(w, r, svcCtx, header.Size) {
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewResourceUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadResource(&course.UploadResourceRequest{
			CourseId:  courseId,
			FileName:  header.Filename,
			FileBytes: data,
		})
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
