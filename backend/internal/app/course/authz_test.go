package course

import (
	"testing"

	"learningplatform/internal/domain/authz"
	"learningplatform/internal/model"
)

// 这些用例锁住"课程是个人资产"这条规则：
// 之前任何登录用户都能删别人的课，是最严重的越权漏洞。
func TestAuthorize_CourseOwnership(t *testing.T) {
	c, err := Store.AddCourse("鉴权测试课程", "", "", "owner_teacher")
	if err != nil {
		t.Fatalf("准备课程失败: %v", err)
	}

	owner := &model.SessionUser{Username: "owner_teacher", Role: "teacher"}
	otherTeacher := &model.SessionUser{Username: "other_teacher", Role: "teacher"}
	student := &model.SessionUser{Username: "student1", Role: "student"}
	admin := &model.SessionUser{Username: "root", Role: "admin"}

	cases := []struct {
		name string
		user *model.SessionUser
		cap  authz.Capability
		want int // 期望的 HTTP 状态码，0 表示放行
	}{
		{"创建者可以编辑", owner, authz.CourseEdit, 0},
		{"创建者可以删除", owner, authz.CourseDelete, 0},
		{"创建者可以上传", owner, authz.CourseUpload, 0},
		{"其他老师不能编辑", otherTeacher, authz.CourseEdit, 403},
		{"其他老师不能删除", otherTeacher, authz.CourseDelete, 403},
		{"学生不能删除", student, authz.CourseDelete, 403},
		{"学生不能上传", student, authz.CourseUpload, 403},
		{"管理员可以删除", admin, authz.CourseDelete, 0},
		{"未登录", nil, authz.CourseDelete, 401},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Authorize(c.Id, tc.user, tc.cap)
			if tc.want == 0 {
				if err != nil {
					t.Fatalf("期望放行，实际报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("期望拒绝，却放行了")
			}
			be, ok := err.(*BizError)
			if !ok {
				t.Fatalf("期望 *BizError，实际 %T", err)
			}
			if be.Code != tc.want {
				t.Fatalf("期望状态码 %d，实际 %d", tc.want, be.Code)
			}
		})
	}
}

// 课程不存在时应是 404，而不是 500 或 403。
func TestAuthorize_NotFound(t *testing.T) {
	u := &model.SessionUser{Username: "someone", Role: "teacher"}
	err := Authorize(999999, u, authz.CourseEdit)
	be, ok := err.(*BizError)
	if !ok || be.Code != 404 {
		t.Fatalf("期望 404，实际 %#v", err)
	}
}
