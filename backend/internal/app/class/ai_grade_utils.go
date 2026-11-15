package class

import (
	"fmt"
	"strings"

	"learningplatform/internal/model"
	"learningplatform/internal/platform/aiservice"
)

// MaxAiGradeItems 一次批改送进 LLM 的主观题上限。
// 和 ai-service 侧 app/api/grade.py 的 MAX_ITEMS 必须一致：
// 那边超了会静默截断，这边超了直接拒绝，避免出现「批了 30 道、成绩册按 35 道算」。
const MaxAiGradeItems = 30

// DefaultAiGradeMaxScore 题目没配分值时给 AI 的默认满分。
const DefaultAiGradeMaxScore = 100

// EssayQuestions 挑出需要 AI 批改的主观题。
// fileupload / fill 不算：附件 AI 读不了，填空题有标准答案走客观判分。
func EssayQuestions(questions []*model.QuestionRow) []*model.QuestionRow {
	out := make([]*model.QuestionRow, 0, len(questions))
	for _, q := range questions {
		if q != nil && q.Type == "essay" {
			out = append(out, q)
		}
	}
	return out
}

// BuildGradeItems 组装送批改的载荷（纯函数，便于单测）。
//
// 学生作答的取法：
//  1. 优先 answers[题目id] —— 作业详情页每道主观题一个文本框，就存这里；
//  2. 整份只有 1 道主观题且没写答案时，退回 content 正文 ——
//     兼容「一个大文本框交全文」的老交法，不然这份作业永远批不了。
//
// 没作答的题直接跳过：AI 打 0 分没有任何信息量，白烧额度。
func BuildGradeItems(questions []*model.QuestionRow, answers map[string]string, content string) []aiservice.GradeItem {
	essays := EssayQuestions(questions)
	content = strings.TrimSpace(content)
	items := make([]aiservice.GradeItem, 0, len(essays))
	for _, q := range essays {
		ans := strings.TrimSpace(answers[fmt.Sprintf("%d", q.Id)])
		if ans == "" && len(essays) == 1 {
			ans = content
		}
		if ans == "" {
			continue
		}
		maxScore := q.Score
		if maxScore <= 0 {
			maxScore = DefaultAiGradeMaxScore
		}
		items = append(items, aiservice.GradeItem{
			QuestionId:     q.Id,
			Stem:           strings.TrimSpace(q.Stem),
			StudentAnswer:  ans,
			StandardAnswer: strings.TrimSpace(q.Answer),
			MaxScore:       maxScore,
		})
	}
	if len(items) > MaxAiGradeItems {
		items = items[:MaxAiGradeItems]
	}
	return items
}

// AiGradeStatusText 状态 → 给人看的中文（前端兜底用，接口也会返回）。
func AiGradeStatusText(status string) string {
	switch status {
	case "":
		return "未发起"
	case model.AIGradeProcessing:
		return "AI 批改中"
	case model.AIGradeDone:
		return "AI 初评完成"
	case model.AIGradeManual:
		return "AI 未能批改，请人工批改"
	case model.AIGradeFailed:
		return "AI 批改失败"
	default:
		return status
	}
}
