package class

import (
	"net/http"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 管理员：用户管理 ----

func listAllUsers(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		if cu.role != "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "仅管理员可查看用户列表"})
			return
		}
		users, err := svcCtx.UserStore.ListAll()
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, users)
	}
}

// applyTeacher 学生申请成为老师
func applyTeacher(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		if err := svcCtx.UserStore.ApplyTeacher(cu.username); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "申请已提交，等待管理员审批"})
	}
}

// updateUserRole 管理员修改用户角色（审批老师申请用）
func updateUserRole(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		if cu.role != "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "仅管理员可修改用户角色"})
			return
		}
		var req struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		}
		body(r, &req)
		if req.Role != "admin" && req.Role != "teacher" && req.Role != "student" {
			httpx.WriteJson(w, 400, map[string]string{"message": "角色不合法"})
			return
		}
		if err := svcCtx.UserStore.UpdateRole(req.Username, req.Role); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}
