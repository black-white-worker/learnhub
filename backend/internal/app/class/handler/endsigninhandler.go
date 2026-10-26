package handler

import (
	"net/http"

	"github.com/zeromicro/go-zero/rest/httpx"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"
)

// EndSigninHandler POST /api/class/:id/signin/end
func EndSigninHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		err := logic.NewEndSigninLogic(r.Context(), svcCtx).EndSignin(classId, cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "签到已结束"})
	}
}
