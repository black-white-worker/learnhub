package model

// 成绩册的行结构与百分比口径。
//
// 为什么把 GradeRow 放在这里而不是 class.go：
// 成绩册的逻辑已经拆成 "取原始数字（SQL）→ 算百分比与加权（gradebook 包）"，
// 这一层只描述"一行成绩包含哪些字段"，和班级其它查询解耦。

// GradeRow 成绩册一行：一个学生在班级里的各类得分率与加权总分。
type GradeRow struct {
	UserId   int64  `db:"user_id" json:"userId"`
	Username string `db:"username" json:"username"`
	Role     string `db:"role" json:"role"`

	// 作业类：得分 / 满分（0 表示该类还没布置过）
	AssignmentScore float64 `db:"assign_score" json:"assignmentScore"`
	AssignmentMax   float64 `db:"assign_max" json:"assignmentMax"`
	// 考试类
	ExamScore float64 `db:"exam_score" json:"examScore"`
	ExamMax   float64 `db:"exam_max" json:"examMax"`
	// 签到：已签次数 / 应签次数
	SigninCnt   int `db:"signin_cnt" json:"signinCnt"`
	SigninTotal int `db:"signin_total" json:"signinTotal"`
	// 视频：已看完数 / 班级绑定课程的视频总数
	VideoDone  int `db:"video_done" json:"videoDone"`
	VideoTotal int `db:"video_total" json:"videoTotal"`

	// ===== 以下为计算字段（由 logic 层用 gradebook 包填充，不参与 SQL 扫描）=====
	AssignmentPercent float64 `db:"-" json:"assignmentPercent"`
	ExamPercent       float64 `db:"-" json:"examPercent"`
	SigninPercent     float64 `db:"-" json:"signinPercent"`
	VideoPercent      float64 `db:"-" json:"videoPercent"`
	TotalScore        float64 `db:"-" json:"totalScore"`
	Letter            string  `db:"-" json:"letter"`

	// 兼容旧前端字段（作业平均分展示）
	AvgScore      float64 `db:"-" json:"avgScore"`
	AssignmentCnt int     `db:"-" json:"assignmentCnt"`
}

// ToGradeRow 把聚合原始行转成 GradeRow（只做字段搬运，不做计算）。
func (r *GradebookRaw) ToGradeRow() *GradeRow {
	return &GradeRow{
		UserId:          r.UserId,
		Username:        r.Username,
		Role:            r.Role,
		AssignmentScore: r.AssignScore,
		AssignmentMax:   r.AssignMax,
		ExamScore:       r.ExamScore,
		ExamMax:         r.ExamMax,
		SigninCnt:       r.SigninCnt,
		SigninTotal:     r.SigninTotal,
		VideoDone:       r.VideoDone,
		VideoTotal:      r.VideoTotal,
		AssignmentCnt:   r.AssignCnt,
	}
}
