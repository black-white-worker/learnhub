package class

import (
	"fmt"
	"strings"

	"learningplatform/internal/model"
)

// GradeAutoAnswers 纯函数：遍历题目，比对学生作答与标准答案，返回客观题自动得分。
// essay（主观题）不参与自动判分，由老师手批。
// multiple（多选题）：答案是逗号分隔的字母集合（如 "A,C"），排序后比较，全等才得分。
func GradeAutoAnswers(questions []*model.QuestionRow, answers map[string]string) int {
	autoScore := 0
	for _, q := range questions {
		if q.Type == "essay" {
			continue
		}
		pick, ok := answers[fmt.Sprintf("%d", q.Id)]
		if !ok {
			continue
		}
		if q.Type == "multiple" {
			if SameMultiChoice(pick, q.Answer) {
				autoScore += q.Score
			}
			continue
		}
		if strings.EqualFold(pick, q.Answer) {
			autoScore += q.Score
		}
	}
	return autoScore
}

// SameMultiChoice 比较两个逗号分隔的选项字母集合，忽略顺序和大小写。
// "A,C" 与 "C,a" 视为相同；"A,B" 与 "A" 或 "A,C" 不同（必须完全一致）。
// 导出供 exam 包复用。
func SameMultiChoice(a, b string) bool {
	parse := func(s string) []string {
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(strings.ToUpper(p))
			if p != "" {
				out = append(out, p)
			}
		}
		sortStrings(out)
		return out
	}
	pa, pb := parse(a), parse(b)
	if len(pa) != len(pb) {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return false
		}
	}
	return true
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
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
