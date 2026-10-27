package handler

import (
	"net/http"
	"strconv"

	"learningplatform/internal/app/course/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListHandler 课程分页列表：GET /api/course/list?page=1&pageSize=12
func ListHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		l := logic.NewListLogic(r.Context(), svcCtx)
		resp, err := l.List(page, size)
		if err != nil {
			courseError(r, w, err)
			return
		}
		httpx.OkJsonCtx(r.Context(), w, resp)
	}
}
