package class

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
	"github.com/zeromicro/go-zero/rest/pathvar"
)

// rand6Code 生成6位数字随机码（签到码/邀请码通用）
func rand6Code() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return fmt.Sprintf("%06d", n.Int64())
}

// RegisterRoutes 注册班级模块所有路由（全部需要登录）
func RegisterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	routes := []rest.Route{
		// 班级基础
		{Method: http.MethodPost, Path: "/api/class/create", Handler: createClass(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/my", Handler: myClasses(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/all", Handler: allClasses(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id", Handler: classDetail(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id", Handler: deleteClass(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/join", Handler: joinClass(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/members", Handler: listMembers(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id/members/:userId", Handler: removeMember(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/courses", Handler: bindCourse(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id/courses/:courseId", Handler: unbindCourse(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/invite", Handler: createInvite(serverCtx)},

		// 公告
		{Method: http.MethodPost, Path: "/api/class/:id/announcement", Handler: createAnnouncement(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/announcements", Handler: listAnnouncements(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/announcement/:aid", Handler: deleteAnnouncement(serverCtx)},

		// 作业
		{Method: http.MethodPost, Path: "/api/class/:id/assignment", Handler: createAssignment(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/assignments", Handler: listAssignments(serverCtx)},
		{Method: http.MethodGet, Path: "/api/assignment/:aid", Handler: assignmentDetail(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/:aid/submit", Handler: submitAssignment(serverCtx)},
		{Method: http.MethodGet, Path: "/api/assignment/:aid/submissions", Handler: listSubmissions(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/submission/:sid/grade", Handler: gradeSubmission(serverCtx)},

		// 讨论
		{Method: http.MethodPost, Path: "/api/class/:id/discussion", Handler: createDiscussion(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/discussions", Handler: listDiscussions(serverCtx)},
		{Method: http.MethodGet, Path: "/api/discussion/:did", Handler: discussionDetail(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/discussion/:did", Handler: deleteDiscussion(serverCtx)},
		{Method: http.MethodPost, Path: "/api/discussion/:did/reply", Handler: replyDiscussion(serverCtx)},

		// 签到
		{Method: http.MethodPost, Path: "/api/class/:id/signin/start", Handler: startSignin(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/signin", Handler: getActiveSignin(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/signin/do", Handler: doSignin(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/signin/records", Handler: listSigninRecords(serverCtx)},

		// 管理员：用户管理
		{Method: http.MethodGet, Path: "/api/admin/users", Handler: listAllUsers(serverCtx)},
		{Method: http.MethodPost, Path: "/api/user/apply-teacher", Handler: applyTeacher(serverCtx)},
		{Method: http.MethodPost, Path: "/api/admin/users/role", Handler: updateUserRole(serverCtx)},
	}

	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, routes...),
	)
}

// ---- 工具函数 ----

type ctxUser struct {
	username string
	role     string
	id       int64
}

// currentUser 从请求取当前登录用户，并查出 user_id
func currentUser(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext) (*ctxUser, bool) {
	u := user.CurrentUser(r)
	if u == nil {
		httpx.WriteJson(w, http.StatusUnauthorized, map[string]string{"message": "未登录"})
		return nil, false
	}
	uid, err := svcCtx.UserStore.GetIdByUsername(u.Username)
	if err != nil {
		httpx.WriteJson(w, http.StatusInternalServerError, map[string]string{"message": "用户不存在"})
		return nil, false
	}
	// 旧 session 可能没有 role 字段（V6 之前登录的），从数据库补查
	role := u.Role
	if role == "" {
		if dbUser := svcCtx.UserStore.GetUser(u.Username); dbUser != nil {
			role = dbUser.Role
		}
	}
	return &ctxUser{username: u.Username, role: role, id: uid}, true
}

// requireClassMember 校验当前用户是该班成员，返回角色
func requireClassMember(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, classId int64, cu *ctxUser) (string, bool) {
	if cu.role == "admin" {
		return "admin", true
	}
	role, err := svcCtx.ClassStore.GetUserClassRole(classId, cu.id)
	if err != nil {
		httpx.WriteJson(w, http.StatusInternalServerError, map[string]string{"message": err.Error()})
		return "", false
	}
	if role == "" {
		httpx.WriteJson(w, http.StatusForbidden, map[string]string{"message": "你不在这个班级"})
		return "", false
	}
	return role, true
}

// requireTeacher 校验当前用户是该班老师或管理员
func requireTeacher(w http.ResponseWriter, r *http.Request, svcCtx *svc.ServiceContext, classId int64, cu *ctxUser) bool {
	role, ok := requireClassMember(w, r, svcCtx, classId, cu)
	if !ok {
		return false
	}
	return role == "teacher" || role == "admin"
}

// parseIntPath 从 go-zero 路径参数取 int。
// go-zero v1.10+ 把 :xxx 路径参数存在 context 的 pathvar 里，不是标准库 r.PathValue，
// 所以必须用 pathvar.Vars(r)；旧代码用 r.PathValue 永远取到空，只能 fallback 取 URL 末段，
// 对 /api/class/2/announcements 这种带子路径的路由会把 "announcements" 当 id 解析成 0，
// 进而 requireClassMember(0, uid) 误判"你不在这个班级"。
func parseIntPath(r *http.Request, key string) int64 {
	// 1. 优先走 go-zero 官方 pathvar（:id / :userId / :aid 等）
	if vars := pathvar.Vars(r); vars != nil {
		if s := vars[key]; s != "" {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil {
				return n
			}
		}
	}
	// 2. 兜底：从 URL 里找第一个纯数字段（防御性，正常不会走到这）
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	for _, p := range parts {
		if n, err := strconv.ParseInt(p, 10, 64); err == nil {
			return n
		}
	}
	return 0
}

func body(r *http.Request, v interface{}) {
	json.NewDecoder(r.Body).Decode(v)
}

// ---- 班级基础 ----

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
		// 创建者自动加入班级当班主任
		_ = svcCtx.ClassStore.AddMember(id, cu.id, "teacher")
		// 创建时自动生成一个邀请码
		code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
		_ = svcCtx.ClassStore.CreateInvitation(id, code)
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id, "inviteCode": code})
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
			"class":     c,
			"members":   members,
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
		// 生成6位随机邀请码
		code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
		if err := svcCtx.ClassStore.CreateInvitation(classId, code); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"code": code})
	}
}

// ---- 公告 ----

func createAnnouncement(svcCtx *svc.ServiceContext) http.HandlerFunc {
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
			Title   string `json:"title"`
			Content string `json:"content"`
			IsTop   bool   `json:"isTop"`
		}
		body(r, &req)
		id, err := svcCtx.ClassStore.InsertAnnouncement(&model.AnnouncementRow{
			ClassId: classId, Title: req.Title, Content: req.Content,
			PublisherId: cu.id, IsTop: req.IsTop,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func listAnnouncements(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, err := svcCtx.ClassStore.ListAnnouncements(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, list)
	}
}

func deleteAnnouncement(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(w, r, svcCtx); !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		_ = svcCtx.ClassStore.DeleteAnnouncement(aid)
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

// ---- 作业 ----

func createAssignment(svcCtx *svc.ServiceContext) http.HandlerFunc {
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
			Title       string `json:"title"`
			Description string `json:"description"`
			Deadline    string `json:"deadline"`
			TotalScore  int    `json:"totalScore"`
			CourseId    int64  `json:"courseId"`
		}
		body(r, &req)
		var dl sql.NullTime
		if req.Deadline != "" {
			if t, err := time.Parse("2006-01-02T15:04", req.Deadline); err == nil {
				dl = sql.NullTime{Time: t, Valid: true}
			}
		}
		score := req.TotalScore
		if score <= 0 {
			score = 100
		}
		id, err := svcCtx.ClassStore.InsertAssignment(&model.AssignmentRow{
			ClassId: classId, Title: req.Title, Description: req.Description,
			Deadline: dl, TotalScore: score, PublisherId: cu.id, CourseId: req.CourseId,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func listAssignments(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, err := svcCtx.ClassStore.ListAssignments(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, list)
	}
}

func assignmentDetail(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, a.ClassId, cu); !ok {
			return
		}
		// 学生看自己的提交
		sub, _ := svcCtx.ClassStore.GetMySubmission(aid, cu.id)
		httpx.WriteJson(w, 200, map[string]interface{}{"assignment": a, "mySubmission": sub})
	}
}

func submitAssignment(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		// 截止时间校验
		if a.Deadline.Valid && a.Deadline.Time.Before(time.Now()) {
			httpx.WriteJson(w, 400, map[string]string{"message": "已超过截止时间，不能提交"})
			return
		}
		var req struct {
			Content string `json:"content"`
			FileUrl string `json:"fileUrl"`
		}
		body(r, &req)
		err = svcCtx.ClassStore.UpsertSubmission(&model.SubmissionRow{
			AssignmentId: aid, UserId: cu.id, Content: req.Content, FileUrl: req.FileUrl,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "提交成功"})
	}
}

func listSubmissions(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if !requireTeacher(w, r, svcCtx, a.ClassId, cu) {
			return
		}
		list, _ := svcCtx.ClassStore.ListSubmissions(aid)
		httpx.WriteJson(w, 200, list)
	}
}

func gradeSubmission(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := currentUser(w, r, svcCtx); !ok {
			return
		}
		sid := parseIntPath(r, "sid")
		var req struct {
			Score   int    `json:"score"`
			Comment string `json:"comment"`
		}
		body(r, &req)
		if err := svcCtx.ClassStore.GradeSubmission(sid, req.Score, req.Comment); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "批改成功"})
	}
}

// ---- 讨论 ----

func createDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		var req struct {
			Title   string `json:"title"`
			Content string `json:"content"`
		}
		body(r, &req)
		id, err := svcCtx.ClassStore.InsertDiscussion(&model.DiscussionRow{
			ClassId: classId, UserId: cu.id, Title: req.Title, Content: req.Content,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func listDiscussions(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, _ := svcCtx.ClassStore.ListDiscussions(classId, 20, 0)
		httpx.WriteJson(w, 200, list)
	}
}

func discussionDetail(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "帖子不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, d.ClassId, cu); !ok {
			return
		}
		replies, _ := svcCtx.ClassStore.ListReplies(did)
		httpx.WriteJson(w, 200, map[string]interface{}{"discussion": d, "replies": replies})
	}
}

func deleteDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err == nil {
			if !requireTeacher(w, r, svcCtx, d.ClassId, cu) {
				return
			}
		}
		_ = svcCtx.ClassStore.DeleteDiscussion(did)
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

func replyDiscussion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		did := parseIntPath(r, "did")
		d, err := svcCtx.ClassStore.GetDiscussion(did)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "帖子不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, d.ClassId, cu); !ok {
			return
		}
		var req struct {
			Content string `json:"content"`
		}
		body(r, &req)
		_, err = svcCtx.ClassStore.InsertReply(&model.ReplyRow{
			DiscussionId: did, UserId: cu.id, Content: req.Content,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "ok"})
	}
}

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

// ---- 签到 ----

// startSignin 老师发起签到：生成6位码，5分钟有效
func startSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		code := rand6Code()
		expires := time.Now().Add(5 * time.Minute)
		id, err := svcCtx.ClassStore.CreateSignin(&model.SigninRow{
			ClassId: classId, TeacherId: cu.id, Code: code, ExpiresAt: expires,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id, "code": code, "expiresAt": expires})
	}
}

// getActiveSignin 查当前进行中的签到（学生端显示签到码输入框；老师端显示当前码）
func getActiveSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		s, err := svcCtx.ClassStore.GetActiveSignin(classId)
		if err != nil {
			httpx.WriteJson(w, 200, nil)
			return
		}
		httpx.WriteJson(w, 200, s)
	}
}

// doSignin 学生提交签到码
func doSignin(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		var req struct {
			Code string `json:"code"`
		}
		body(r, &req)
		s, err := svcCtx.ClassStore.GetSigninByCode(strings.TrimSpace(req.Code))
		if err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": "签到码无效"})
			return
		}
		if s.ClassId != classId {
			httpx.WriteJson(w, 400, map[string]string{"message": "签到码不属于本班级"})
			return
		}
		if s.ExpiresAt.Before(time.Now()) {
			httpx.WriteJson(w, 400, map[string]string{"message": "签到已结束"})
			return
		}
		if err := svcCtx.ClassStore.AddSigninRecord(s.Id, cu.id); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "签到成功"})
	}
}

// listSigninRecords 老师查看已签名单
func listSigninRecords(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		s, err := svcCtx.ClassStore.GetActiveSignin(classId)
		if err != nil {
			httpx.WriteJson(w, 200, []interface{}{})
			return
		}
		list, err := svcCtx.ClassStore.ListSigninRecords(s.Id)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, list)
	}
}
