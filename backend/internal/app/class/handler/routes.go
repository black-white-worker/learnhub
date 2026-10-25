// Package handler 班级模块 HTTP 入口层：薄壳 handler + 路由注册。
// 同包内每个接口对应一个 xxxHandler 函数，内部调 logic 包。
package handler

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
		{Method: http.MethodPost, Path: "/api/class/create", Handler: createClassHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/my", Handler: myClassesHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/all", Handler: allClassesHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id", Handler: classDetailHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id", Handler: deleteClassHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/join", Handler: joinClassHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/members", Handler: listMembersHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id/members/:userId", Handler: removeMemberHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/courses", Handler: bindCourseHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/:id/courses/:courseId", Handler: unbindCourseHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/invite", Handler: createInviteHandler(serverCtx)},

		// 公告
		{Method: http.MethodPost, Path: "/api/class/:id/announcement", Handler: createAnnouncementHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/announcements", Handler: listAnnouncementsHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/class/announcement/:aid", Handler: deleteAnnouncementHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/announcement/:aid/read", Handler: markAnnouncementReadHandler(serverCtx)},

		// 作业
		{Method: http.MethodPost, Path: "/api/class/:id/assignment", Handler: createAssignmentHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/assignments", Handler: listAssignmentsHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/assignment/:aid", Handler: assignmentDetailHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/:aid/questions", Handler: addQuestionHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/:aid/submit", Handler: submitAssignmentHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/upload", Handler: uploadAssignmentFileHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/assignment/:aid/submissions", Handler: listSubmissionsHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/assignment/submission/:sid/grade", Handler: gradeSubmissionHandler(serverCtx)},

		// 讨论
		{Method: http.MethodPost, Path: "/api/class/:id/discussion", Handler: createDiscussionHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/discussions", Handler: listDiscussionsHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/discussion/:did", Handler: discussionDetailHandler(serverCtx)},
		{Method: http.MethodDelete, Path: "/api/discussion/:did", Handler: deleteDiscussionHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/discussion/:did/reply", Handler: replyDiscussionHandler(serverCtx)},

		// 签到
		{Method: http.MethodPost, Path: "/api/class/:id/signin/start", Handler: startSigninHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/signin", Handler: getActiveSigninHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/class/:id/signin/do", Handler: doSigninHandler(serverCtx)},
		{Method: http.MethodGet, Path: "/api/class/:id/signin/records", Handler: listSigninRecordsHandler(serverCtx)},

		// 管理员：用户管理
		{Method: http.MethodGet, Path: "/api/admin/users", Handler: listAllUsersHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/user/apply-teacher", Handler: applyTeacherHandler(serverCtx)},
		{Method: http.MethodPost, Path: "/api/admin/users/role", Handler: updateUserRoleHandler(serverCtx)},

		// 成绩与学情
		{Method: http.MethodGet, Path: "/api/class/:id/grades", Handler: listGradesHandler(serverCtx)},
	}

	server.AddRoutes(
		rest.WithMiddleware(user.NewAuthMiddleware(serverCtx).Handle, routes...),
	)
}
