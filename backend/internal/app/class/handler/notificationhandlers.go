package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// listNotificationsHandler GET /api/notifications
func listNotificationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		resp, err := logic.NewListNotificationsLogic(r.Context(), svcCtx).ListNotifications(cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, resp)
	}
}

// markNotificationReadHandler POST /api/notifications/:id/read
func markNotificationReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		id := class.ParseIntPath(r, "id")
		if err := logic.NewMarkNotificationReadLogic(r.Context(), svcCtx).MarkOne(id, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "已读"})
	}
}

// markAllNotificationsReadHandler POST /api/notifications/read-all
func markAllNotificationsReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		if err := logic.NewMarkNotificationReadLogic(r.Context(), svcCtx).MarkAll(cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "全部已读"})
	}
}
