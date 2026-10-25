package logic

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"learningplatform/internal/app/class"
	"learningplatform/internal/svc"

	"github.com/google/uuid"
)

type UploadAssignmentLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUploadAssignmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UploadAssignmentLogic {
	return &UploadAssignmentLogic{ctx: ctx, svcCtx: svcCtx}
}

// Upload 处理作业附件上传：校验大小/扩展名 → 传 MinIO → 返回访问 URL。
// fileBytes 由 handler 从 multipart 表单读出后传入。
func (l *UploadAssignmentLogic) Upload(filename string, fileBytes []byte, cu *class.CtxUser) (*class.UploadAssignmentResp, error) {
	if l.svcCtx.Oss == nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "对象存储未初始化，请检查 MinIO 配置"}
	}
	if len(fileBytes) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "文件内容为空"}
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if _, ok := class.AllowedAssignmentExt[ext]; !ok {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "不支持的文件类型"}
	}
	objectName := fmt.Sprintf("assignment/%d/%s%s", cu.Id, uuid.NewString(), ext)
	url, err := l.svcCtx.Oss.PutObject(l.ctx, objectName, bytes.NewReader(fileBytes),
		int64(len(fileBytes)), "application/octet-stream")
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "上传失败：" + err.Error()}
	}
	return &class.UploadAssignmentResp{Url: url, Name: filename, Size: int64(len(fileBytes)), ObjectName: objectName}, nil
}

var _ = io.LimitReader
