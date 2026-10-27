package authz

import (
	"errors"
	"net/http"
	"testing"
)

// 断言：矩阵里最关键的几条规则，防止后续改动把"越权"放回来。
func TestCan_CourseOwnership(t *testing.T) {
	owner := Actor{UserId: 1, Username: "alice", Role: "teacher"}
	other := Actor{UserId: 2, Username: "bob", Role: "teacher"}
	admin := Actor{UserId: 3, Username: "root", Role: "admin"}
	student := Actor{UserId: 4, Username: "stu", Role: "student"}

	res := Resource{OwnerUsername: "alice"}

	cases := []struct {
		name string
		act  Actor
		cap  Capability
		res  Resource
		want bool
	}{
		{"创建者可以编辑自己的课", owner, CourseEdit, res, true},
		{"创建者可以删除自己的课", owner, CourseDelete, res, true},
		{"创建者可以上传资源", owner, CourseUpload, res, true},
		{"其他老师不能编辑别人的课", other, CourseEdit, res, false},
		{"其他老师不能删除别人的课", other, CourseDelete, res, false},
		{"其他老师不能往别人的课上传", other, CourseUpload, res, false},
		{"管理员可以编辑任何课", admin, CourseDelete, res, true},
		{"学生不能编辑课程", student, CourseEdit, res, false},
		{"用户名缺失时不视为创建者", Actor{Username: "", Role: "teacher"}, CourseEdit, res, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.act.Can(c.cap, c.res); got != c.want {
				t.Fatalf("Can(%s)=%v，期望 %v", c.cap, got, c.want)
			}
		})
	}
}

// 断言：班级范围内的能力必须看班内角色，全局 teacher 不够。
func TestCan_ClassScoped(t *testing.T) {
	globalTeacherOutsider := Actor{UserId: 1, Username: "t1", Role: "teacher", ClassRole: ""}
	globalTeacherStudent := Actor{UserId: 2, Username: "t2", Role: "teacher", ClassRole: "student"}
	classTeacher := Actor{UserId: 3, Username: "t3", Role: "student", ClassRole: "teacher"}
	admin := Actor{UserId: 4, Username: "root", Role: "admin"}

	cases := []struct {
		name string
		act  Actor
		want bool
	}{
		{"全局老师但不是班级成员", globalTeacherOutsider, false},
		{"全局老师却是该班学生", globalTeacherStudent, false},
		{"全局学生但是该班老师", classTeacher, true},
		{"管理员", admin, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, cap := range []Capability{ExamGrade, AssignmentGrade, ClassManage, GradeViewAll} {
				if got := c.act.Can(cap, Resource{ClassRole: c.act.ClassRole}); got != c.want {
					t.Fatalf("%s: Can(%s)=%v，期望 %v", c.name, cap, got, c.want)
				}
			}
		})
	}
}

// 断言：未声明的能力 fail-closed。
func TestCan_UnknownCapabilityDenied(t *testing.T) {
	a := Actor{Role: "teacher"}
	if a.Can(Capability("something.unregistered"), Resource{}) {
		t.Fatal("未注册能力必须默认拒绝")
	}
}

// 断言：Require 返回的错误能被 response 包按 403 处理。
func TestRequire_DeniedStatus(t *testing.T) {
	student := Actor{Role: "student"}
	err := student.Require(CourseEdit, Resource{OwnerUsername: "alice"})
	if err == nil {
		t.Fatal("期望拒绝，却通过了")
	}
	var coded interface{ StatusCode() int }
	if !errors.As(err, &coded) {
		t.Fatal("拒绝错误应实现 StatusCode()，供 response 输出 403")
	}
	if coded.StatusCode() != http.StatusForbidden {
		t.Fatalf("状态码应为 403，实际 %d", coded.StatusCode())
	}
	if err.Error() == "" {
		t.Fatal("拒绝原因不能为空，前端要直接展示")
	}
}

// 断言：有权限时 Require 返回 nil。
func TestRequire_Allowed(t *testing.T) {
	owner := Actor{Username: "alice", Role: "teacher"}
	if err := owner.Require(CourseDelete, Resource{OwnerUsername: "alice"}); err != nil {
		t.Fatalf("创建者应被放行，实际报错: %v", err)
	}
}
