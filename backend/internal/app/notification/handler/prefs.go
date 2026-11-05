// Package handler 通知中心 HTTP 层。
package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/notification"
	"learningplatform/internal/app/notification/logic"
	"learningplatform/internal/platform/response"
	"learningplatform/internal/svc"
)

// listPrefsHandler GET /api/notification-prefs
// 返回当前用户的通知渠道偏好列表。
// 用 response.OK 统一包成 {code,message,data}，前端按 res.data.data.prefs 取值。
func listPrefsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		resp, err := logic.NewPrefsLogic(r.Context(), svcCtx).List(cu)
		if err != nil {
			class.WriteBizError(w, err)
			return
		}
		response.OK(w, resp)
	}
}

// updatePrefsHandler POST /api/notification-prefs
// 新增或更新单个渠道的偏好开关。
func updatePrefsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		var req notification.UpdatePrefReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, &class.BizError{Code: 400, Msg: "参数解析失败"})
			return
		}
		if err := logic.NewPrefsLogic(r.Context(), svcCtx).Update(cu, req.Channel, req.Enabled); err != nil {
			class.WriteBizError(w, err)
			return
		}
		response.OK(w, map[string]string{"message": "ok"})
	}
}
