package handler

import (
	"net/http"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/class/logic"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func updateUserRoleHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, be := class.CurrentUser(r, svcCtx)
		if be != nil {
			class.WriteBizError(w, be)
			return
		}
		var req class.UpdateUserRoleReq
		if err := class.ParseJSON(r, &req); err != nil {
			class.WriteBizError(w, err)
			return
		}
		if err := logic.NewUpdateUserRoleLogic(r.Context(), svcCtx).UpdateUserRole(&req, cu); err != nil {
			class.WriteBizError(w, err)
			return
		}
		// 角色已变更：立即让该用户的角色缓存失效，避免还要等 60 秒才生效
		user.InvalidateRoleCache(svcCtx.Redis, req.Username)
		httpx.WriteJson(w, http.StatusOK, map[string]string{"message": "ok"})
	}
}
