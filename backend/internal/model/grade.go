package model

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ===== 成绩册（对标 Canvas gradebook）=====
//
// 设计要点：成绩不是查询时现算，而是"提交/批改时写入 grade_scores"，
// 查询只做聚合。这样：
//   - 权重调整、补交、重批都只影响一条记录，不会牵动整个统计口径；
//   - 导出成绩单 / 学生查自己成绩可以直接复用同一份数据；
//   - 视频、签到这类"无提交动作"的项，仍然按需聚合，不强行造记录。

// GradeItemRow 成绩项：一个作业或一场考试。
type GradeItemRow struct {
	Id         int64     `db:"id" json:"id"`
	ClassId    int64     `db:"class_id" json:"classId"`
	SourceType string    `db:"source_type" json:"sourceType"`
	SourceId   int64     `db:"source_id" json:"sourceId"`
	Title      string    `db:"title" json:"title"`
	MaxScore   float64   `db:"max_score" json:"maxScore"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
}

// GradeScoreRow 一条成绩记录。
type GradeScoreRow struct {
	Id       int64     `db:"id" json:"id"`
	ItemId   int64     `db:"item_id" json:"itemId"`
	UserId   int64     `db:"user_id" json:"userId"`
	Score    float64   `db:"score" json:"score"`
	GradedBy int64     `db:"graded_by" json:"gradedBy"`
	GradedAt time.Time `db:"graded_at" json:"gradedAt"`
}

// EnsureGradeItem 幂等地建成绩项（作业/考试创建或首次提交时调用）。
// 已存在则不覆盖 max_score/title？——需要覆盖：老师可能把总分从 100 改成 60。
func (m *ClassModel) EnsureGradeItem(classId int64, sourceType string, sourceId int64, title string, maxScore float64) (int64, error) {
	if maxScore <= 0 {
		maxScore = 100
	}
	ctx := context.Background()
	if _, err := m.conn.ExecCtx(ctx,
		`INSERT INTO grade_items (class_id, source_type, source_id, title, max_score)
		 VALUES (?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE class_id=VALUES(class_id), title=VALUES(title), max_score=VALUES(max_score)`,
		classId, sourceType, sourceId, title, maxScore); err != nil {
		return 0, err
	}
	var id int64
	if err := m.conn.QueryRowCtx(ctx, &id,
		"SELECT id FROM grade_items WHERE source_type=? AND source_id=?", sourceType, sourceId); err != nil {
		return 0, err
	}
	return id, nil
}

// SetGradeScore 写入/更新一条成绩（幂等）。gradedBy=0 表示系统自动判分。
func (m *ClassModel) SetGradeScore(itemId, userId int64, score float64, gradedBy int64) error {
	_, err := m.conn.ExecCtx(context.Background(),
		`INSERT INTO grade_scores (item_id, user_id, score, graded_by)
		 VALUES (?,?,?,?)
		 ON DUPLICATE KEY UPDATE score=VALUES(score), graded_by=VALUES(graded_by), graded_at=NOW()`,
		itemId, userId, score, gradedBy)
	return err
}

// GetGradeScore 读一条成绩（不存在返回 nil, nil）。
func (m *ClassModel) GetGradeScore(itemId, userId int64) (*GradeScoreRow, error) {
	var s GradeScoreRow
	err := m.conn.QueryRowCtx(context.Background(), &s,
		"SELECT id, item_id, user_id, score, graded_by, graded_at FROM grade_scores WHERE item_id=? AND user_id=?",
		itemId, userId)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GradebookRaw 成绩册聚合的原始行（百分比在校验后用纯函数算，见 gradebook 包）。
type GradebookRaw struct {
	UserId      int64   `db:"user_id"`
	Username    string  `db:"username"`
	Role        string  `db:"role"`
	AssignScore float64 `db:"assign_score"`
	AssignMax   float64 `db:"assign_max"`
	AssignCnt   int     `db:"assign_cnt"`
	ExamScore   float64 `db:"exam_score"`
	ExamMax     float64 `db:"exam_max"`
	SigninCnt   int     `db:"signin_cnt"`
	SigninTotal int     `db:"signin_total"`
	VideoDone   int     `db:"video_done"`
	VideoTotal  int     `db:"video_total"`
}

// ListGradebookRaw 一次查询拿到成绩册需要的全部原始数字。
//
// 说明：签到次数、视频完成数用相关子查询聚合，行数 = 班级人数（通常 < 100），
// 子查询都走索引；相比在 Go 里 for 循环逐个学生查（N+1），这里是 1 次往返。
func (m *ClassModel) ListGradebookRaw(classId int64) ([]*GradebookRaw, error) {
	var rows []*GradebookRaw
	err := m.conn.QueryRowsCtx(context.Background(), &rows, `
		SELECT cm.user_id AS user_id,
		       u.username   AS username,
		       cm.role      AS role,
		       COALESCE(SUM(CASE WHEN gi.source_type='assignment' THEN gs.score    ELSE 0 END), 0) AS assign_score,
		       COALESCE(SUM(CASE WHEN gi.source_type='assignment' THEN gi.max_score ELSE 0 END), 0) AS assign_max,
		       COALESCE(SUM(CASE WHEN gi.source_type='assignment' AND gs.id IS NOT NULL THEN 1 ELSE 0 END), 0) AS assign_cnt,
		       COALESCE(SUM(CASE WHEN gi.source_type='exam'       THEN gs.score    ELSE 0 END), 0) AS exam_score,
		       COALESCE(SUM(CASE WHEN gi.source_type='exam'       THEN gi.max_score ELSE 0 END), 0) AS exam_max,
		       (SELECT COUNT(*) FROM signin_records r
		          JOIN class_signin cs ON r.signin_id = cs.id
		         WHERE cs.class_id = cm.class_id AND r.user_id = cm.user_id) AS signin_cnt,
		       (SELECT COUNT(*) FROM class_signin cs WHERE cs.class_id = cm.class_id) AS signin_total,
		       (SELECT COUNT(*) FROM video_progress vp
		          JOIN course_videos cv ON cv.id = vp.video_id
		          JOIN class_courses cc  ON cc.course_id = cv.course_id
		         WHERE cc.class_id = cm.class_id AND vp.user_id = cm.user_id AND vp.completed = 1) AS video_done,
		       (SELECT COUNT(*) FROM course_videos cv
		          JOIN class_courses cc ON cc.course_id = cv.course_id
		         WHERE cc.class_id = cm.class_id) AS video_total
		  FROM class_members cm
		  JOIN users u ON u.id = cm.user_id
		  LEFT JOIN grade_scores gs ON gs.user_id = cm.user_id
		  LEFT JOIN grade_items  gi ON gi.id = gs.item_id AND gi.class_id = cm.class_id
		 WHERE cm.class_id = ?
		 GROUP BY cm.user_id, u.username, cm.role, cm.class_id
		 ORDER BY u.username ASC`, classId)
	return rows, err
}

// GetGradeItemBySource 取某作业/考试对应的成绩项 id（不存在返回 0）。
func (m *ClassModel) GetGradeItemBySource(sourceType string, sourceId int64) (int64, error) {
	var id int64
	err := m.conn.QueryRowCtx(context.Background(), &id,
		"SELECT id FROM grade_items WHERE source_type=? AND source_id=?", sourceType, sourceId)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// RuleMap 把权重转成前端/接口用的 map（成绩册表头展示"这次按什么权重算的"）。
func RuleMap(video, assignment, signin, exam int) map[string]int {
	return map[string]int{
		"video":      video,
		"assignment": assignment,
		"signin":     signin,
		"exam":       exam,
	}
}

// CountClassStudents 班级学生数（成绩单导出表头用）。
func (m *ClassModel) CountClassStudents(classId int64) (int64, error) {
	var n int64
	err := m.conn.QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM class_members WHERE class_id=? AND role='student'", classId)
	return n, err
}
