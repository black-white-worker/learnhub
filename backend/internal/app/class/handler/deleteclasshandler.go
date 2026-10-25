package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func deleteClassHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		if err := logic.NewDeleteClassLogic(r.Context(), svcCtx).DeleteClass(classId, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "ok"})
	}
}
