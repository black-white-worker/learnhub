package course

import (
	"testing"

	"learningplatform/internal/model"
)

// 媒体访问规则表：这是"私有桶不再被匿名绕过"的核心判定，必须穷举。
func TestDecideMediaAccess(t *testing.T) {
	student := &model.SessionUser{Username: "stu", Role: "student"}
	teacher := &model.SessionUser{Username: "tea", Role: "teacher"}
	otherTeacher := &model.SessionUser{Username: "other", Role: "teacher"}
	admin := &model.SessionUser{Username: "root", Role: "admin"}

	cases := []struct {
		name         string
		actor        *model.SessionUser
		owner        string
		boundClasses int
		myClasses    int
		wantAllowed  bool
		wantOpen     bool
	}{
		// 未登录：任何课程都不给看媒体
		{"匿名访问独立课", nil, "tea", 0, 0, false, false},
		{"匿名访问班级课", nil, "tea", 2, 0, false, false},

		// 管理员 / 创建者
		{"管理员", admin, "tea", 3, 0, true, false},
		{"创建者本人", teacher, "tea", 3, 0, true, false},

		// 独立课程：登录即可
		{"学生访问独立课", student, "tea", 0, 0, true, true},
		{"课程无归属也能看", student, "", 0, 0, true, true},

		// 班级课：必须是成员
		{"学生是班级成员", student, "tea", 2, 1, true, false},
		{"学生不是班级成员", student, "tea", 2, 0, false, false},
		{"别的班的学生", otherTeacher, "tea", 2, 0, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DecideMediaAccess(c.actor, c.owner, c.boundClasses, c.myClasses)
			if got.Allowed != c.wantAllowed {
				t.Fatalf("Allowed=%v，期望 %v（reason=%q）", got.Allowed, c.wantAllowed, got.Reason)
			}
			if got.Open != c.wantOpen {
				t.Fatalf("Open=%v，期望 %v", got.Open, c.wantOpen)
			}
			// 拒绝必须给出可展示的原因，否则前端只能显示"加载失败"
			if !got.Allowed && got.Reason == "" {
				t.Fatal("拒绝时必须给出可读原因")
			}
		})
	}
}

// ResolveMediaAccess 在未登录 / 课程为空时快速拒绝，不依赖任何外部存储。
func TestResolveMediaAccess_NoActorOrCourse(t *testing.T) {
	if got := ResolveMediaAccess(nil, nil, nil); got.Allowed {
		t.Fatal("未登录且无课程时不应放行")
	}
	if got := ResolveMediaAccess(nil, &model.SessionUser{Username: "stu"}, nil); got.Allowed {
		t.Fatal("课程为 nil 时不应放行")
	}
}

// 创建者 / 管理员不查库即可放行（单测里 svcCtx 为 nil 也能通过）。
func TestResolveMediaAccess_OwnerShortCircuit(t *testing.T) {
	actor := &model.SessionUser{Username: "tea", Role: "teacher"}
	c := &Course{Id: 1, CreatedBy: "tea"}
	if got := ResolveMediaAccess(nil, actor, c); !got.Allowed {
		t.Fatalf("创建者应放行，实际: %+v", got)
	}
	admin := &model.SessionUser{Username: "root", Role: "admin"}
	if got := ResolveMediaAccess(nil, admin, c); !got.Allowed {
		t.Fatalf("管理员应放行，实际: %+v", got)
	}
}
