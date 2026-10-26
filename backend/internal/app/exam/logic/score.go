package logic

import (
	"sort"
	"strconv"
	"strings"

	"learningplatform/internal/model"
)

// ScoreResult 判分结果。
type ScoreResult struct {
	// AutoScore 客观题自动得分（单选/多选/判断）
	AutoScore int
	// UnansweredIDs 学生没作答的客观题 id（前端可提示）
	UnansweredIDs []int64
	// WrongIDs 判错的客观题 id
	WrongIDs []int64
}

// ScoreExam 客观题自动判分纯函数。
// 抽出来是为了：1) 可单测；2) 后续 AI 模块做主观题初评时复用。
//
// 判分规则：
//   - single：学生答案 == 标准答案 得满分
//   - judge：同上
//   - multiple：学生选的集合 == 标准答案集合才给分（不做部分给分，教学版简化）
//   - essay：跳过（老师手批）
//   - 学生没作答：不计分，记到 UnansweredIDs
func ScoreExam(questions []*model.ExamQuestionRow, answers map[string]string) ScoreResult {
	res := ScoreResult{}
	for _, q := range questions {
		if q.Type == "essay" {
			continue
		}
		pick, ok := answers[strconv.FormatInt(q.Id, 10)]
		if !ok || strings.TrimSpace(pick) == "" {
			res.UnansweredIDs = append(res.UnansweredIDs, q.Id)
			continue
		}
		hit := false
		switch q.Type {
		case "multiple":
			hit = sameMultiChoice(pick, q.Answer)
		default: // single / judge
			hit = strings.EqualFold(strings.TrimSpace(pick), strings.TrimSpace(q.Answer))
		}
		if hit {
			res.AutoScore += q.Score
		} else {
			res.WrongIDs = append(res.WrongIDs, q.Id)
		}
	}
	return res
}

// sameMultiChoice 比较两个多选答案是否一致。
// 答案格式约定："A,C,B" 或 "["A","C","B"]" 都行，按字母集合排序后比较。
// 兼容 class.SameMultiChoice 的语义，但这里不直接依赖 class 包，避免循环 import。
func sameMultiChoice(pick, answer string) bool {
	pSet := splitChoice(pick)
	aSet := splitChoice(answer)
	if len(pSet) != len(aSet) {
		return false
	}
	sort.Strings(pSet)
	sort.Strings(aSet)
	for i := range pSet {
		if !strings.EqualFold(pSet[i], aSet[i]) {
			return false
		}
	}
	return true
}

// splitChoice 把 "A,C" 或 `["A","C"]` 拆成 ["A","C"]，去空白、去引号。
func splitChoice(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "[]")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, `"'`)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
