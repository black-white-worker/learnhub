package course

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// CreateHandler 课程创建接口的 HTTP 处理器：
// 解析请求体 → 调用业务逻辑 → 返回 JSON
func CreateHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. 解析请求体（JSON：name / intro / teacher）
		var req CreateRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		// 2. 调用课程创建业务逻辑
		l := NewCreateLogic(r.Context(), svcCtx)
		resp, err := l.Create(&req)
		if err != nil {
			// 创建失败（名称为空 / 课程已存在）
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			// 创建成功，返回新课程信息
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
