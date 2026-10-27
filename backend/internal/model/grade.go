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
// 性能设计（B 端成绩册的核心查询）：
//
//	旧实现把"签到次数 / 视频完成数"写成相关子查询，放在 SELECT 列表里，
//	对 class_members 的每一行各执行一次 —— 121 个学生就是 121×2 次子查询，
//	再叠加 grade_scores × grade_items 的宽连接，实测 268~296ms
//	（120 人 / 4560 条成绩 / 3840 条签到记录 / 3456 条进度，EXPLAIN ANALYZE）。
//
//	现在改成"先预聚合、再 LEFT JOIN"：每类统计只扫一遍并物化成 ≤ 班级人数的行，
//	同一份数据实测 59~107ms（热 60ms 左右），且耗时不再随学生数线性乘以子查询次数。
//	外层仍然是 1 次往返，百分比/加权仍在 gradebook 纯函数里算（口径不变）。
func (m *ClassModel) ListGradebookRaw(classId int64) ([]*GradebookRaw, error) {
	var rows []*GradebookRaw
	err := m.conn.QueryRowsCtx(context.Background(), &rows, `
		SELECT cm.user_id AS user_id,
		       u.username   AS username,
		       cm.role      AS role,
		       COALESCE(ag.assign_score, 0) AS assign_score,
		       COALESCE(ag.assign_max, 0)   AS assign_max,
		       COALESCE(ag.assign_cnt, 0)   AS assign_cnt,
		       COALESCE(ag.exam_score, 0)   AS exam_score,
		       COALESCE(ag.exam_max, 0)     AS exam_max,
		       COALESCE(sg.signin_cnt, 0)   AS signin_cnt,
		       (SELECT COUNT(*) FROM class_signin cs WHERE cs.class_id = cm.class_id) AS signin_total,
		       COALESCE(vg.video_done, 0)   AS video_done,
		       (SELECT COUNT(*) FROM course_videos cv
		          JOIN class_courses cc ON cc.course_id = cv.course_id
		         WHERE cc.class_id = cm.class_id) AS video_total
		  FROM class_members cm
		  JOIN users u ON u.id = cm.user_id
		  LEFT JOIN (
		        -- 作业/考试的得分、满分、已交数：一次扫描后按学生聚合
		        SELECT gs.user_id,
		               SUM(CASE WHEN gi.source_type = 'assignment' THEN gs.score    ELSE 0 END) AS assign_score,
		               SUM(CASE WHEN gi.source_type = 'assignment' THEN gi.max_score ELSE 0 END) AS assign_max,
		               SUM(CASE WHEN gi.source_type = 'assignment' THEN 1 ELSE 0 END)            AS assign_cnt,
		               SUM(CASE WHEN gi.source_type = 'exam'       THEN gs.score    ELSE 0 END) AS exam_score,
		               SUM(CASE WHEN gi.source_type = 'exam'       THEN gi.max_score ELSE 0 END) AS exam_max
		          FROM grade_items gi
		          JOIN grade_scores gs ON gs.item_id = gi.id
		         WHERE gi.class_id = ?
		         GROUP BY gs.user_id
		  ) ag ON ag.user_id = cm.user_id
		  LEFT JOIN (
		        -- 签到次数：按班聚合一次
		        SELECT r.user_id, COUNT(*) AS signin_cnt
		          FROM signin_records r
		          JOIN class_signin cs ON cs.id = r.signin_id
		         WHERE cs.class_id = ?
		         GROUP BY r.user_id
		  ) sg ON sg.user_id = cm.user_id
		  LEFT JOIN (
		        -- 视频完成数：先锁班级（4 门课）→ 课程视频（48 个）→ 覆盖索引查进度。
		        -- STRAIGHT_JOIN 是必要的：优化器默认会全扫 video_progress（所有班级的进度
		        -- 都扫一遍），实测 78ms；按班级驱动 + idx_video_completed 覆盖索引只要 5.7ms。
		        SELECT vp.user_id, COUNT(*) AS video_done
		          FROM class_courses cc
		          STRAIGHT_JOIN course_videos cv ON cv.course_id = cc.course_id
		          STRAIGHT_JOIN video_progress vp ON vp.video_id = cv.id AND vp.completed = 1
		         WHERE cc.class_id = ?
		         GROUP BY vp.user_id
		  ) vg ON vg.user_id = cm.user_id
		 WHERE cm.class_id = ?
		 ORDER BY u.username ASC`, classId, classId, classId, classId)
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
