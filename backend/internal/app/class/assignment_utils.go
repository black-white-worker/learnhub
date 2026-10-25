package class

import (
	"fmt"
	"strings"

	"learningplatform/internal/model"
)

// GradeAutoAnswers 纯函数：遍历题目，比对学生作答与标准答案，返回客观题自动得分。
// essay（主观题）不参与自动判分，由老师手批。
func GradeAutoAnswers(questions []*model.QuestionRow, answers map[string]string) int {
	autoScore := 0
	for _, q := range questions {
		if q.Type == "essay" {
			continue
		}
		pick, ok := answers[fmt.Sprintf("%d", q.Id)]
		if ok && strings.EqualFold(pick, q.Answer) {
			autoScore += q.Score
		}
	}
	return autoScore
}

// ValidateSubmissionInput 纯函数：校验作业提交内容不能整包为空。
func ValidateSubmissionInput(content, fileUrl string, answers map[string]string) error {
	hasText := strings.TrimSpace(content) != ""
	hasFile := strings.TrimSpace(fileUrl) != ""
	hasAnswer := len(answers) > 0
	if !hasText && !hasFile && !hasAnswer {
		return fmt.Errorf("提交内容不能为空：请填写作业文本、上传附件或作答客观题")
	}
	return nil
}

// AllowedAssignmentExt 允许学生交作业时上传的附件扩展名白名单
var AllowedAssignmentExt = map[string]struct{}{
	".pdf": {}, ".doc": {}, ".docx": {}, ".ppt": {}, ".pptx": {},
	".xls": {}, ".xlsx": {}, ".zip": {}, ".rar": {}, ".7z": {},
	".jpg": {}, ".jpeg": {}, ".png": {}, ".txt": {}, ".md": {},
}

// MaxAssignmentBytes 作业附件大小上限：50MB
const MaxAssignmentBytes = 50 << 20
