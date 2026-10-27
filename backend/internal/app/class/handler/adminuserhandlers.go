package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// adminOnly 校验当前登录用户是否为管理员，不是则返回 403。
func adminOnly(w http.ResponseWriter, r *http.Request) bool {
	u := user.CurrentUser(r)
	if u == nil {
		httpx.WriteJson(w, 401, map[string]string{"message": "未登录"})
		return false
	}
	if u.Role != "admin" {
		httpx.WriteJson(w, 403, map[string]string{"message": "仅管理员可操作"})
		return false
	}
	return true
}

// ===== POST /api/admin/users  新增用户 =====

type adminCreateUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

func AdminCreateUserHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminOnly(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req adminCreateUserReq
		_ = json.Unmarshal(body, &req)
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" || req.Password == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "用户名和密码不能为空"})
			return
		}
		if req.Role != "admin" && req.Role != "teacher" && req.Role != "student" {
			req.Role = "student"
		}
		if req.Nickname == "" {
			req.Nickname = req.Username
		}
		if err := svcCtx.UserStore.Add(req.Username, req.Password); err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": err.Error()})
			return
		}
		_ = svcCtx.UserStore.UpdateRole(req.Username, req.Role)
		_ = svcCtx.UserStore.UpdateNickname(req.Username, req.Nickname)
		httpx.OkJsonCtx(r.Context(), w, map[string]any{"username": req.Username, "role": req.Role})
	}
}

// ===== PUT /api/admin/users  编辑用户（昵称+角色） =====

type adminUpdateUserReq struct {
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Role     string `json:"role"`
}

func AdminUpdateUserHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminOnly(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req adminUpdateUserReq
		_ = json.Unmarshal(body, &req)
		if req.Username == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "缺少用户名"})
			return
		}
		cu := user.CurrentUser(r)
		if cu.Username == req.Username {
			httpx.WriteJson(w, 400, map[string]string{"message": "不能修改自己的账号"})
			return
		}
		if req.Nickname != "" {
			_ = svcCtx.UserStore.UpdateNickname(req.Username, req.Nickname)
		}
		if req.Role == "admin" || req.Role == "teacher" || req.Role == "student" {
			_ = svcCtx.UserStore.UpdateRole(req.Username, req.Role)
			_ = svcCtx.UserStore.ClearApplyTeacher(req.Username)
			// 角色变更立即生效：清掉 60 秒的角色缓存
			user.InvalidateRoleCache(svcCtx.Redis, req.Username)
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
	}
}

// ===== DELETE /api/admin/users  删除用户（query: username） =====

func AdminDeleteUserHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminOnly(w, r) {
			return
		}
		username := r.URL.Query().Get("username")
		if username == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "缺少用户名"})
			return
		}
		cu := user.CurrentUser(r)
		if cu.Username == username {
			httpx.WriteJson(w, 400, map[string]string{"message": "不能删除自己"})
			return
		}
		if err := svcCtx.UserStore.Delete(username); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
	}
}

// ===== POST /api/admin/users/reset-password  重置密码 =====

type adminResetPwdReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func AdminResetPasswordHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !adminOnly(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req adminResetPwdReq
		_ = json.Unmarshal(body, &req)
		if req.Username == "" || req.Password == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "用户名和新密码不能为空"})
			return
		}
		if err := svcCtx.UserStore.ResetPassword(req.Username, req.Password); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
	}
}

var _ = class.BizError{}
