// Package authz 统一授权判定（对标 Moodle 的 capability 模型）。
//
// 背景：改造前权限判断散落在 handler 和 logic 里，
// 有的模块写了 RequireTeacher（class），有的只判了"已登录"（course），
// 导致同一类操作在不同模块有不同安全水位，漏一个 if 就是一个越权漏洞。
//
// 本包把「谁（Actor）能对什么（Resource）做什么（Capability）」
// 收敛成一张静态矩阵：判定只写在一处，加新能力只需要在 can 里加一行，
// 调用方统一用 Require，把"系统性遗漏"降级成"局部改动"。
//
// 设计约束：
//   - 只依赖标准库，不 import 业务包（避免循环依赖）；
//   - 拒绝时返回 *Denied，它实现了 StatusCode() int，
//     与 internal/response 的 codedError 鸭子类型约定兼容，
//     因此不需要 import response / class 也能输出正确的 403。
package authz

import "net/http"

// Capability 一个权限点（能力）。命名参照 Moodle：动词化的「资源.动作」。
type Capability string

const (
	// CourseCreate 创建课程（老师 / 管理员）
	CourseCreate Capability = "course.create"
	// CourseEdit 编辑课程基本信息（课程创建者 / 管理员）
	CourseEdit Capability = "course.edit"
	// CourseDelete 删除课程（课程创建者 / 管理员）
	CourseDelete Capability = "course.delete"
	// CourseUpload 向课程上传封面 / 视频 / 讲义（课程创建者 / 管理员）
	CourseUpload Capability = "course.upload"
	// CourseResourceDelete 删除课程下的讲义资源（课程创建者 / 管理员）
	CourseResourceDelete Capability = "course.resource.delete"

	// ClassManage 班级管理：改名、踢人、绑课、发公告（班内老师 / 管理员）
	ClassManage Capability = "class.manage"
	// ClassCreate 创建班级（老师 / 管理员）
	ClassCreate Capability = "class.create"

	// AssignmentManage 作业出题 / 改作业设置（班内老师 / 管理员）
	AssignmentManage Capability = "assignment.manage"
	// AssignmentGrade 批改作业（班内老师 / 管理员）
	AssignmentGrade Capability = "assignment.grade"

	// ExamManage 考试出题 / 改设置（班内老师 / 管理员）
	ExamManage Capability = "exam.manage"
	// ExamGrade 批改考试主观题（班内老师 / 管理员）
	ExamGrade Capability = "exam.grade"

	// GradeViewAll 查看全班成绩册（班内老师 / 管理员）
	GradeViewAll Capability = "grade.view.all"
	// AdminUsers 管理全站用户（仅管理员）
	AdminUsers Capability = "admin.users"
)

// Actor 发起操作的主体。
//
// Role 是全局角色（users.role）：admin / teacher / student。
// ClassRole 是资源所属班级内的角色（class_members.role）：
// teacher / student，不在该班级时为空串。两者必须分开看——
// 全局 teacher 也可能是另一个班的 student，此时不应获得批改权限。
type Actor struct {
	UserId    int64
	Username  string
	Role      string
	ClassRole string
}

// Resource 被操作对象的属性，由调用方按需填。
// 只填和目标能力相关的字段；未填的字段一律视为"不满足"。
type Resource struct {
	// OwnerUsername 资源创建者的用户名（courses.created_by）。
	OwnerUsername string
	// ClassRole 资源所在班级内，当前 Actor 的角色（"teacher"/"student"/""）。
	ClassRole string
}

// IsAdmin 是否全站管理员。
func (a Actor) IsAdmin() bool { return a.Role == "admin" }

// IsTeacher 是否全站老师（不含管理员）。
func (a Actor) IsTeacher() bool { return a.Role == "teacher" }

// IsClassTeacher 是否是该班级的老师（admin 也算，admin 在班内等同老师）。
func (a Actor) IsClassTeacher() bool {
	return a.Role == "admin" || a.ClassRole == "teacher"
}

// isOwner 判断是否资源创建者本人。用户名比较，空用户名视为不匹配。
func (a Actor) isOwner(res Resource) bool {
	return res.OwnerUsername != "" && a.Username != "" && res.OwnerUsername == a.Username
}

// Can 纯函数：判断 Actor 是否拥有某能力。这是整个授权体系的唯一事实来源。
func (a Actor) Can(cap Capability, res Resource) bool {
	// 管理员对所有能力放行（Moodle 里 admin 是 site-level 全能力角色）
	if a.IsAdmin() {
		return true
	}

	switch cap {
	case CourseCreate, ClassCreate:
		// 全站老师可以建课 / 建班；学生不行（学生可申请成为老师）
		return a.IsTeacher()

	case CourseEdit, CourseDelete, CourseUpload, CourseResourceDelete:
		// 课程属于个人资产：只有创建者本人可改，其余老师也不能动别人的课
		return a.isOwner(res)

	case ClassManage, AssignmentManage, AssignmentGrade, ExamManage, ExamGrade, GradeViewAll:
		// 班级范围内的能力：必须以班内老师身份出现（不是"全局老师"就行）
		return a.IsClassTeacher()

	case AdminUsers:
		return false
	}

	// 未声明的能力默认拒绝（fail-closed），避免新增能力忘了加规则时变成放行
	return false
}

// Denied 授权失败错误。实现 StatusCode() 让 response.WriteError 输出 403 +
// {code:403,message:...}，调用方无需导入 response 包。
type Denied struct {
	Msg string
}

func (d *Denied) Error() string {
	if d.Msg == "" {
		return "没有权限执行该操作"
	}
	return d.Msg
}

// StatusCode 供 response 包识别业务状态码。
func (d *Denied) StatusCode() int { return http.StatusForbidden }

// Require 授权校验：有权限返回 nil，否则返回 *Denied。
//
// 典型用法（logic 层第一行）：
//
//	if err := actor.Require(authz.CourseDelete, authz.Resource{OwnerUsername: c.CreatedBy}); err != nil {
//	    return nil, err
//	}
func (a Actor) Require(cap Capability, res Resource) error {
	if a.Can(cap, res) {
		return nil
	}
	return &Denied{Msg: denyMessage(cap)}
}

// denyMessage 给出可读的拒绝原因，便于前端直接展示。
func denyMessage(cap Capability) string {
	switch cap {
	case CourseEdit, CourseDelete, CourseUpload, CourseResourceDelete:
		return "只有课程创建者可以修改这门课程"
	case ClassManage:
		return "需要班级老师权限"
	case AssignmentManage, AssignmentGrade:
		return "需要班级老师权限才能管理作业"
	case ExamManage, ExamGrade:
		return "需要班级老师权限才能管理考试"
	case GradeViewAll:
		return "需要班级老师权限才能查看成绩册"
	case AdminUsers:
		return "仅管理员可操作"
	case CourseCreate, ClassCreate:
		return "需要老师权限"
	}
	return "没有权限执行该操作"
}
