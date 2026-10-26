// Package exam 考试中心：班级内的限时考试，支持选择/判断/主观题。
package exam

// CreateExamReq 创建考试
type CreateExamReq struct {
	Title          string `json:"title"`
	Description    string `json:"description" form:"description,optional"`
	StartAt        string `json:"startAt"`        // "2006-01-02T15:04"
	EndAt          string `json:"endAt"`          // "2006-01-02T15:04"
	DurationMin    int    `json:"durationMinutes" form:"durationMinutes,optional"`
	TotalScore     int    `json:"totalScore" form:"totalScore,optional"`
}

type CreateExamResp struct {
	Id int64 `json:"id"`
}

// AddQuestionReq 给考试加题
type AddQuestionReq struct {
	Type    string `json:"type"` // single/judge/essay
	Stem    string `json:"stem"`
	Options string `json:"options" form:"options,optional"`
	Answer  string `json:"answer" form:"answer,optional"`
	Score   int    `json:"score" form:"score,optional"`
}

type AddQuestionResp struct {
	Id int64 `json:"id"`
}

// SubmitReq 交卷
type SubmitReq struct {
	Answers map[string]string `json:"answers" form:"answers,optional"`
}

type SubmitResp struct {
	Message   string `json:"message"`
	AutoScore int    `json:"autoScore,omitempty"`
}

// GradeReq 老师手批
type GradeReq struct {
	TeacherScore int    `json:"teacherScore"`
	Comment      string `json:"comment" form:"comment,optional"`
}

// StartExamResp 学生开考响应：返回考试时长和截止时间，前端据此倒计时。
type StartExamResp struct {
	ExamId      int64  `json:"examId"`
	StartedAt   string `json:"startedAt"`   // RFC3339
	DeadlineAt  string `json:"deadlineAt"` // 学生个人交卷截止（= start+duration 与 end_at 的较早者）
	DurationMin int    `json:"durationMinutes"`
}

// BatchAddQuestionsReq 老师批量组卷：一次塞多道题。
type BatchAddQuestionsReq struct {
	Questions []BatchQuestionItem `json:"questions"`
}

// BatchQuestionItem 批量组卷里的单题项，字段与 AddQuestionReq 对齐。
type BatchQuestionItem struct {
	Type    string `json:"type"` // single/multiple/judge/essay
	Stem    string `json:"stem"`
	Options string `json:"options" form:"options,optional"`
	Answer  string `json:"answer" form:"answer,optional"`
	Score   int    `json:"score" form:"score,optional"`
}

type BatchAddQuestionsResp struct {
	Count int    `json:"count"`
	Ids   []int64 `json:"ids"`
}
