package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func createAnnouncementHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		classId := class.ParseIntPath(r, "id")
		var req class.CreateAnnouncementReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		resp, err := logic.NewCreateAnnouncementLogic(r.Context(), svcCtx).CreateAnnouncement(classId, &req, cu, r.RemoteAddr)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}
