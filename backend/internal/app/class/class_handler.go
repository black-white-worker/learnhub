package class

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 班级基础：创建/列表/详情/加入/成员/课程绑定/邀请码 ----

func createClass(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		// 只有老师和管理员能创建班级
		if cu.role != "teacher" && cu.role != "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "只有老师或管理员才能创建班级"})
			return
		}
		var req struct {
			Name  string `json:"name"`
			Grade string `json:"grade"`
			Major string `json:"major"`
		}
		body(r, &req)
		if req.Name == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "班级名不能为空"})
			return
		}
		id, err := svcCtx.ClassStore.InsertClass(req.Name, req.Grade, req.Major, cu.username, cu.id)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		// 创建者自动加入班级当班主任：加成员失败必须回滚并报错，否则创建者会成"隐形人"
		if err := svcCtx.ClassStore.AddMember(id, cu.id, "teacher"); err != nil {
			_ = svcCtx.ClassStore.DeleteClass(id)
			httpx.WriteJson(w, 500, map[string]string{"message": "加入班级失败：" + err.Error()})
			return
		}
		// 创建时自动生成一个邀请码
		code := rand6Code()
		if err := svcCtx.ClassStore.CreateInvitation(id, code); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "邀请码生成失败：" + err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id, "inviteCode": code})
		logOperation(svcCtx, r, cu.id, cu.username, "create_class", fmt.Sprintf("class:%d", id), req.Name)
	}
}

func myClasses(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classes, err := svcCtx.ClassStore.ListClassesByUser(cu.id)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, classes)
	}
}

func allClasses(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		if cu.role != "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "仅管理员可查看所有班级"})
			return
		}
		classes, err := svcCtx.ClassStore.ListAllClasses()
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, classes)
	}
}

func classDetail(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		c, err := svcCtx.ClassStore.GetClass(classId)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "班级不存在"})
			return
		}
		members, _ := svcCtx.ClassStore.ListMembers(classId)
		courseIds, _ := svcCtx.ClassStore.ListClassCourseIds(classId)
		// 取该班最新一条邀请码返回给前端展示（可能为空）
		inviteCode := ""
		if invs, _ := svcCtx.ClassStore.ListInvitations(classId); len(invs) > 0 {
			inviteCode = invs[len(invs)-1].Code
		}
		httpx.WriteJson(w, 200, map[string]interface{}{
			"class":      c,
			"members":    members,
			"courseIds":  courseIds,
			"inviteCode": inviteCode,
		})
	}
}

func deleteClass(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if cu.role != "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "仅管理员可删除班级"})
			return
		}
		if err := svcCtx.ClassStore.DeleteClass(classId); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func joinClass(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		body(r, &req)
		inv, err := svcCtx.ClassStore.GetInvitationByCode(strings.TrimSpace(req.Code))
		if err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": "邀请码无效"})
			return
		}
		if inv.ExpiresAt.Valid && inv.ExpiresAt.Time.Before(time.Now()) {
			httpx.WriteJson(w, 400, map[string]string{"message": "邀请码已过期"})
			return
		}
		if inv.MaxUses > 0 && inv.UsedCount >= inv.MaxUses {
			httpx.WriteJson(w, 400, map[string]string{"message": "邀请码已达使用上限"})
			return
		}
		if err := svcCtx.ClassStore.AddMember(inv.ClassId, cu.id, "student"); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		_ = svcCtx.ClassStore.IncInvitationUse(inv.Id)
		httpx.WriteJson(w, 200, map[string]interface{}{"classId": inv.ClassId, "message": "加入成功"})
	}
}

func listMembers(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		members, err := svcCtx.ClassStore.ListMembers(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, members)
	}
}

func removeMember(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		userId := parseIntPath(r, "userId")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		if err := svcCtx.ClassStore.RemoveMember(classId, userId); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func bindCourse(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		var req struct {
			CourseId int64 `json:"courseId"`
		}
		body(r, &req)
		if err := svcCtx.ClassStore.BindCourse(classId, req.CourseId); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func unbindCourse(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		courseId := parseIntPath(r, "courseId")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		_ = svcCtx.ClassStore.UnbindCourse(classId, courseId)
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func createInvite(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		// 生成6位随机邀请码（crypto/rand，不可预测）
		code := rand6Code()
		if err := svcCtx.ClassStore.CreateInvitation(classId, code); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"code": code})
	}
}
