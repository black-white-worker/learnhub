package model

// 管理员班级总览的 SQL 回归：这段查询全是相关子查询 + LEFT JOIN users，
// 字段名写错或统计口径错了只有在真库上才暴露，所以放集成用例。
// 连不上库自动 skip（与其他 model 集成测试同一套约定）。

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestListAdminClassesAndUpdateInfo(t *testing.T) {
	ctx := context.Background()
	conn := newModelTestConn(t)
	cm := NewClassModel(conn)

	var uid int64
	var uname string
	if err := conn.QueryRowCtx(ctx, &uid, "SELECT id FROM users ORDER BY id LIMIT 1"); err != nil {
		t.Skipf("跳过：库里没有用户（%v）", err)
	}
	_ = conn.QueryRowCtx(ctx, &uname, "SELECT username FROM users WHERE id=?", uid)

	sfx := time.Now().UnixNano() % 1000000
	name := fmt.Sprintf("管理测试班_%d", sfx)
	classId := mustExec(t, conn,
		"INSERT INTO classes (name, grade, major, head_teacher_id, created_by) VALUES (?,?,?,?,?)",
		name, "2026级", "测试", uid, uname)
	t.Cleanup(func() { _, _ = cm.DeleteClass(classId) })

	mustExec(t, conn, "INSERT INTO class_members (class_id, user_id, role) VALUES (?,?,?)", classId, uid, "teacher")
	mustExec(t, conn, "INSERT INTO class_courses (class_id, course_id) VALUES (?,?)", classId, 1)

	rows, err := cm.ListAdminClasses()
	if err != nil {
		t.Fatalf("ListAdminClasses 失败: %v", err)
	}
	var row *AdminClassRow
	for _, r := range rows {
		if r.Id == classId {
			row = r
			break
		}
	}
	if row == nil {
		t.Fatalf("总览里找不到刚建的班级 %d", classId)
	}
	if row.MemberCount != 1 || row.StudentCount != 0 {
		t.Fatalf("成员统计不对: member=%d student=%d", row.MemberCount, row.StudentCount)
	}
	if row.CourseCount != 1 {
		t.Fatalf("课程统计不对: %d", row.CourseCount)
	}
	if row.HeadTeacherName == "" {
		t.Fatal("班主任姓名应回填 username/nickname，实际为空")
	}

	// 改名 + 改年级/专业
	if err := cm.UpdateClassInfo(classId, name+"_改", "2025级", "计算机"); err != nil {
		t.Fatalf("UpdateClassInfo 失败: %v", err)
	}
	got, err := cm.GetClass(classId)
	if err != nil {
		t.Fatalf("改名后读取失败: %v", err)
	}
	if got.Name != name+"_改" || got.Grade != "2025级" || got.Major != "计算机" {
		t.Fatalf("改名未生效: %+v", got)
	}
}
