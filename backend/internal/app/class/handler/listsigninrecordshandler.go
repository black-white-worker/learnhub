package handler

import (
	"net/http"
	"strconv"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func listSigninRecordsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		signinId, _ := strconv.ParseInt(r.URL.Query().Get("signinId"), 10, 64)
		resp, err := logic.NewListSigninRecordsLogic(r.Context(), svcCtx).ListSigninRecords(classId, signinId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}
