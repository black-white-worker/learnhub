// Package class 班级模块路由注册。
// 本文件只做一件事：把所有路由挂到 server 上，并统一套上登录中间件。
// 具体 handler 按业务域拆分到同目录下的 *_handler.go：
//   - common.go              公共工具（currentUser/鉴权/parseIntPath/随机码）
//   - class_handler.go       班级 CRUD、成员、课程绑定、邀请码
//   - announcement_handler.go 公告
//   - assignment_handler.go  作业、提交、客观题、附件上传
//   - discussion_handler.go  讨论、回复
//   - signin_handler.go      签到
//   - admin_handler.go       管理员用户管理
//   - grade_handler.go       成绩聚合
//   - log.go                 操作日志异步落库
package class

import (
	"net/http"

	"learningplatform/internal/app/user"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

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
		{Method: http.MethodPost, Path: "/api/class/announcement/:aid/read", Handler: markAnnouncementRead(serverCtx)},

		// 作业
		{Method: http.MethodPost, Path: "/api/class/:id/assignment", Handler: createAssignment(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/assignments", Handler: listAssignments(serverCtx)},
		{Method: http.MethodGet, Path: "/api/assignment/:aid", Handler: assignmentDetail(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/:aid/questions", Handler: addQuestion(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/:aid/submit", Handler: submitAssignment(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/upload", Handler: uploadAssignmentFile(serverCtx)},
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

		// 成绩与学情
		{Method: http.MethodGet, Path: "/api/class/:id/grades", Handler: listGrades(serverCtx)},
	}

	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, routes...),
	)
}
