package class

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ---- 作业：布置/列表/详情/加题/提交/批改/附件上传 ----

// gradeAutoAnswers 纯函数：遍历题目，比对学生作答与标准答案，返回客观题自动得分。
// 抽出来便于单测（不依赖 DB / HTTP）。key=题目id字符串，value=学生选的字母。
// 比对用 EqualFold 忽略大小写，避免学生答 "a" 和答案 "A" 被判错。
func gradeAutoAnswers(questions []*model.QuestionRow, answers map[string]string) int {
	autoScore := 0
	for _, q := range questions {
		pick, ok := answers[fmt.Sprintf("%d", q.Id)]
		if ok && strings.EqualFold(pick, q.Answer) {
			autoScore += q.Score
		}
	}
	return autoScore
}

// validateSubmissionInput 纯函数：校验作业提交内容不能整包为空。
// 文本内容、附件 URL、客观题作答三者至少要有一项，否则视为空提交。
// 返回 nil 表示合法；返回错误文案供 handler 直接 400 给前端。
func validateSubmissionInput(content, fileUrl string, answers map[string]string) error {
	hasText := strings.TrimSpace(content) != ""
	hasFile := strings.TrimSpace(fileUrl) != ""
	hasAnswer := len(answers) > 0
	if !hasText && !hasFile && !hasAnswer {
		return fmt.Errorf("提交内容不能为空：请填写作业文本、上传附件或作答客观题")
	}
	return nil
}

func createAssignment(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if !requireTeacher(w, r, svcCtx, classId, cu) {
			return
		}
		var req struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Deadline    string `json:"deadline"`
			TotalScore  int    `json:"totalScore"`
			CourseId    int64  `json:"courseId"`
		}
		body(r, &req)
		var dl sql.NullTime
		if req.Deadline != "" {
			if t, err := time.Parse("2006-01-02T15:04", req.Deadline); err == nil {
				dl = sql.NullTime{Time: t, Valid: true}
			}
		}
		score := req.TotalScore
		if score <= 0 {
			score = 100
		}
		id, err := svcCtx.ClassStore.InsertAssignment(&model.AssignmentRow{
			ClassId: classId, Title: req.Title, Description: req.Description,
			Deadline: dl, TotalScore: score, PublisherId: cu.id, CourseId: req.CourseId,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func listAssignments(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		classId := parseIntPath(r, "id")
		if _, ok := requireClassMember(w, r, svcCtx, classId, cu); !ok {
			return
		}
		list, err := svcCtx.ClassStore.ListAssignments(classId)
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, list)
	}
}

func assignmentDetail(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if _, ok := requireClassMember(w, r, svcCtx, a.ClassId, cu); !ok {
			return
		}
		// 题目列表：学生端不返回正确答案字段（Answer 标记了 json:"-"）
		questions, _ := svcCtx.ClassStore.ListQuestions(aid)
		sub, _ := svcCtx.ClassStore.GetMySubmission(aid, cu.id)
		httpx.WriteJson(w, 200, map[string]interface{}{"assignment": a, "questions": questions, "mySubmission": sub})
	}
}

// addQuestion 老师给作业加客观题
func addQuestion(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if !requireTeacher(w, r, svcCtx, a.ClassId, cu) {
			return
		}
		var req struct {
			Type    string `json:"type"`
			Stem    string `json:"stem"`
			Options string `json:"options"`
			Answer  string `json:"answer"`
			Score   int    `json:"score"`
		}
		body(r, &req)
		if req.Stem == "" || req.Answer == "" {
			httpx.WriteJson(w, 400, map[string]string{"message": "题干和答案不能为空"})
			return
		}
		if req.Score <= 0 {
			req.Score = 5
		}
		id, err := svcCtx.ClassStore.InsertQuestion(&model.QuestionRow{
			AssignmentId: aid, Type: req.Type, Stem: req.Stem, Options: req.Options, Answer: req.Answer, Score: req.Score,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{"id": id})
	}
}

func submitAssignment(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		// 校验班级成员身份，同时拿到角色：班级创建者/老师不需要提交作业
		role, ok := requireClassMember(w, r, svcCtx, a.ClassId, cu)
		if !ok {
			return
		}
		if role == "teacher" || role == "admin" {
			httpx.WriteJson(w, 403, map[string]string{"message": "老师/创建者无需提交作业"})
			return
		}
		// 截止时间校验
		if a.Deadline.Valid && a.Deadline.Time.Before(time.Now()) {
			httpx.WriteJson(w, 400, map[string]string{"message": "已超过截止时间，不能提交"})
			return
		}
		var req struct {
			Content string            `json:"content"`
			FileUrl string            `json:"fileUrl"`
			Answers map[string]string `json:"answers"` // key=questionId, value=学生选项
		}
		body(r, &req)
		// 非空校验：文本/附件/客观题至少要有一项
		if err := validateSubmissionInput(req.Content, req.FileUrl, req.Answers); err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": err.Error()})
			return
		}
		err = svcCtx.ClassStore.UpsertSubmission(&model.SubmissionRow{
			AssignmentId: aid, UserId: cu.id, Content: req.Content, FileUrl: req.FileUrl,
		})
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		// 客观题自动判分：遍历题目，比对答案，累计 auto_score
		if len(req.Answers) > 0 {
			questions, _ := svcCtx.ClassStore.ListQuestions(aid)
			autoScore := gradeAutoAnswers(questions, req.Answers)
			answersJSON, _ := json.Marshal(req.Answers)
			sub, _ := svcCtx.ClassStore.GetMySubmission(aid, cu.id)
			if sub != nil {
				_ = svcCtx.ClassStore.UpdateSubmissionAutoScore(sub.Id, string(answersJSON), autoScore)
			}
			httpx.WriteJson(w, 200, map[string]interface{}{"message": "提交成功", "autoScore": autoScore})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "提交成功"})
	}
}

func listSubmissions(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		aid := parseIntPath(r, "aid")
		a, err := svcCtx.ClassStore.GetAssignment(aid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if !requireTeacher(w, r, svcCtx, a.ClassId, cu) {
			return
		}
		list, _ := svcCtx.ClassStore.ListSubmissions(aid)
		httpx.WriteJson(w, 200, list)
	}
}

func gradeSubmission(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		sid := parseIntPath(r, "sid")
		sub, err := svcCtx.ClassStore.GetSubmission(sid)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "提交记录不存在"})
			return
		}
		// 反查作业所在班级，仅该班老师/管理员可批改
		a, err := svcCtx.ClassStore.GetAssignment(sub.AssignmentId)
		if err != nil {
			httpx.WriteJson(w, 404, map[string]string{"message": "作业不存在"})
			return
		}
		if !requireTeacher(w, r, svcCtx, a.ClassId, cu) {
			return
		}
		var req struct {
			Score   int    `json:"score"`
			Comment string `json:"comment"`
		}
		body(r, &req)
		if err := svcCtx.ClassStore.GradeSubmission(sid, req.Score, req.Comment); err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]string{"message": "批改成功"})
	}
}

// ---- 作业附件上传 ----

// allowedAssignmentExt 允许学生交作业时上传的附件扩展名（白名单）。
// 作业通常是文档/压缩包/图片，不允许可执行文件避免安全风险。
var allowedAssignmentExt = map[string]struct{}{
	".pdf":  {},
	".doc":  {},
	".docx": {},
	".ppt":  {},
	".pptx": {},
	".xls":  {},
	".xlsx": {},
	".zip":  {},
	".rar":  {},
	".7z":   {},
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".txt":  {},
	".md":   {},
}

// maxAssignmentBytes 作业附件大小上限：50MB
const maxAssignmentBytes = 50 << 20

// uploadAssignmentFile 学生交作业时上传附件，直接传 MinIO，返回可访问 URL。
// 路由：POST /api/assignment/upload（multipart 字段 file）
// 注意：本接口只校验登录，不绑定具体作业——学生先传附件拿到 URL，再在 submit 接口里带上 fileUrl。
func uploadAssignmentFile(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cu, ok := currentUser(w, r, svcCtx)
		if !ok {
			return
		}
		if svcCtx.Oss == nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "对象存储未初始化，请检查 MinIO 配置"})
			return
		}
		if err := r.ParseMultipartForm(maxAssignmentBytes); err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": "表单解析失败"})
			return
		}
		defer r.MultipartForm.RemoveAll()

		file, header, err := r.FormFile("file")
		if err != nil {
			httpx.WriteJson(w, 400, map[string]string{"message": "请上传文件（字段名 file）"})
			return
		}
		defer file.Close()

		if header.Size > maxAssignmentBytes {
			httpx.WriteJson(w, 400, map[string]string{"message": "附件超过 50MB 上限"})
			return
		}
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if _, ok := allowedAssignmentExt[ext]; !ok {
			httpx.WriteJson(w, 400, map[string]string{"message": "不支持的文件类型"})
			return
		}

		data, err := io.ReadAll(io.LimitReader(file, maxAssignmentBytes+1))
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "读取文件失败"})
			return
		}
		if len(data) == 0 {
			httpx.WriteJson(w, 400, map[string]string{"message": "文件内容为空"})
			return
		}

		// 对象名按学生维度组织：assignment/<userId>/<uuid><ext>，避免同名覆盖
		objectName := fmt.Sprintf("assignment/%d/%s%s", cu.id, uuid.NewString(), ext)
		url, err := svcCtx.Oss.PutObject(r.Context(), objectName, bytes.NewReader(data),
			int64(len(data)), "application/octet-stream")
		if err != nil {
			httpx.WriteJson(w, 500, map[string]string{"message": "上传失败：" + err.Error()})
			return
		}
		httpx.WriteJson(w, 200, map[string]interface{}{
			"url":         url,
			"name":        header.Filename,
			"size":        header.Size,
			"objectName":  objectName,
		})
	}
}
