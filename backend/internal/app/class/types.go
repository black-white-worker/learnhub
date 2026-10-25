package class

import (
	"learningplatform/internal/model"
)

// BizError 业务错误：带 HTTP 状态码，handler 据此写响应。
// logic 层只 return &BizError{Code:403, Msg:"..."}，不碰 http.ResponseWriter。
type BizError struct {
	Code int
	Msg  string
}

func (e *BizError) Error() string { return e.Msg }

// ---- 班级 ----

type CreateClassReq struct {
	Name  string `json:"name"`
	Grade string `json:"grade"`
	Major string `json:"major"`
}
type CreateClassResp struct {
	Id         int64  `json:"id"`
	InviteCode string `json:"inviteCode"`
}

type ClassDetailResp struct {
	Class      *model.ClassRow        `json:"class"`
	Members    []*model.ClassMemberRow `json:"members"`
	CourseIds  []int64                `json:"courseIds"`
	InviteCode string                 `json:"inviteCode"`
	// MyRole 当前登录用户在本班的角色（teacher/student/admin/空=不在班），
	// 由 logic 层用 RequireClassMember 的返回值填充，前端不用自己再去 members 里 find。
	MyRole string `json:"myRole"`
}

type JoinClassReq struct {
	Code string `json:"code"`
}
type JoinClassResp struct {
	ClassId int64  `json:"classId"`
	Message string `json:"message"`
}

type BindCourseReq struct {
	CourseId int64 `json:"courseId"`
}

type CreateInviteResp struct {
	Code string `json:"code"`
}

// ---- 公告 ----

type CreateAnnouncementReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	// IsTop 可选，默认 false。go-zero 的 httpx.Parse 对 bool 零值敏感，
	// 不加 optional 会在前端不传该字段时报 "field isTop is not set"。
	IsTop bool `json:"isTop" form:"isTop,optional"`
}
type CreateAnnouncementResp struct {
	Id int64 `json:"id"`
}

// AnnouncementWithRead 公告列表项：在 AnnouncementRow 基础上附加已读状态
type AnnouncementWithRead struct {
	*model.AnnouncementRow
	ReadByMe  bool `json:"readByMe"`
	ReadCount int  `json:"readCount"`
}

// ---- 作业 ----

type CreateAssignmentReq struct {
	Title       string `json:"title"`
	Description string `json:"description" form:"description,optional"`
	// Deadline 格式 "2006-01-02T15:04"，可选，不传=无截止时间
	Deadline string `json:"deadline" form:"deadline,optional"`
	// TotalScore 可选，<=0 时 logic 层默认 100
	TotalScore int `json:"totalScore" form:"totalScore,optional"`
	// CourseId 可选，0=不关联课程
	CourseId int64 `json:"courseId" form:"courseId,optional"`
}
type CreateAssignmentResp struct {
	Id int64 `json:"id"`
}

type AssignmentDetailResp struct {
	Assignment   *model.AssignmentRow `json:"assignment"`
	Questions    []*model.QuestionRow `json:"questions"`
	MySubmission *model.SubmissionRow `json:"mySubmission"`
}

type AddQuestionReq struct {
	Type    string `json:"type" form:"type,optional"`
	Stem    string `json:"stem"`
	// Options 选择题为 JSON 数组字符串，判断题为空，故可选
	Options string `json:"options" form:"options,optional"`
	Answer  string `json:"answer"`
	// Score 可选，<=0 时 logic 层默认 5
	Score int `json:"score" form:"score,optional"`
}
type AddQuestionResp struct {
	Id int64 `json:"id"`
}

type SubmitAssignmentReq struct {
	// Content/FileUrl/Answers 三者至少填一个，由 logic 层 ValidateSubmissionInput 校验。
	// 前端可能只传 answers（客观题）或只传 fileUrl（附件），故都标 optional，
	// 否则 go-zero 会因为 string 零值 "" 而报 "content is not set"。
	Content string            `json:"content" form:"content,optional"`
	FileUrl string            `json:"fileUrl" form:"fileUrl,optional"`
	Answers map[string]string `json:"answers" form:"answers,optional"`
}
type SubmitAssignmentResp struct {
	Message   string `json:"message"`
	AutoScore int    `json:"autoScore,omitempty"`
}

type GradeSubmissionReq struct {
	Score   int    `json:"score"`
	Comment string `json:"comment" form:"comment,optional"`
}

type UploadAssignmentResp struct {
	Url        string `json:"url"`
	Name       string `json:"name"`
	Size       int64  `json:"size"`
	ObjectName string `json:"objectName"`
}

// ---- 讨论 ----

type CreateDiscussionReq struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}
type CreateDiscussionResp struct {
	Id int64 `json:"id"`
}

type DiscussionDetailResp struct {
	Discussion *model.DiscussionRow `json:"discussion"`
	Replies    []*model.ReplyRow    `json:"replies"`
}

type ReplyReq struct {
	Content string `json:"content"`
}

// ---- 签到 ----

type DoSigninReq struct {
	Code string `json:"code"`
}

// ---- 管理员 ----

type UpdateUserRoleReq struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

// ---- 成绩 ----

type ListGradesResp struct {
	Rule map[string]int      `json:"rule"`
	Rows []*model.GradeRow    `json:"rows"`
}
