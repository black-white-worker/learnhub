package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"learningplatform/internal/app/course"
	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// CoverUploadHandler 封面上传接口的 HTTP 处理器：
// 解析 multipart 表单 → 读出封面文件字节 → 调用业务逻辑 → 返回 JSON。
//
// 前端调用示例（FormData）：
//
//	formData.append("courseId", "1")
//	formData.append("file", coverFile)
//	POST /api/course/upload/cover
func CoverUploadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
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
		// 前端没填课程名时不会带合法 courseId；这里兜底返回中文友好错误，
		// 而不是让后面的"课程不存在"或框架默认英文错误暴露给用户
		if courseId <= 0 {
			httpx.ErrorCtx(r.Context(), w, errors.New("请先输入课程名称并创建课程，再上传封面"))
			return
		}

		// 3. 封面文件（表单字段 file，单文件）
		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, errors.New("请上传封面文件（字段名 file）"))
			return
		}
		defer file.Close()

		// 4. 大小预检（逻辑层还会按配置再校验一次）
		if header.Size > course.EffectiveMaxCoverBytes(svcCtx) {
			httpx.ErrorCtx(r.Context(), w, errors.New("封面文件超过大小上限"))
			return
		}

		// 5. 读取文件字节（限制读取长度，防止超限文件撑爆内存）
		data, err := io.ReadAll(io.LimitReader(file, course.EffectiveMaxCoverBytes(svcCtx)+1))
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 6. 调用封面上传业务逻辑
		l := logic.NewCoverUploadLogic(r.Context(), svcCtx)
		resp, err := l.UploadCover(&course.UploadCoverRequest{
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
