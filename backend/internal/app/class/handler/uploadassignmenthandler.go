package handler

import (
	"io"
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// uploadAssignmentFileHandler 作业附件上传（multipart）。
// multipart 解析无法走 httpx.Parse，这里手动读表单后调 logic。
func uploadAssignmentFileHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		if err := r.ParseMultipartForm(class.MaxAssignmentBytes); err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "表单解析失败"})
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "请上传文件（字段名 file）"})
			return
		}
		defer file.Close()

		if header.Size > class.MaxAssignmentBytes {
			class.WriteBizError(w, &class.BizError{Code: http.StatusBadRequest, Msg: "附件超过 50MB 上限"})
			return
		}
		data, err := io.ReadAll(io.LimitReader(file, class.MaxAssignmentBytes+1))
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewUploadAssignmentLogic(r.Context(), svcCtx).Upload(header.Filename, data, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}
