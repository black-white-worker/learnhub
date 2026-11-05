package logic

import (
	"math"
	"sort"
	"strconv"
	"strings"

	"learningplatform/internal/model"
)

// ScoreResult 判分结果。
type ScoreResult struct {
	// AutoScore 客观题自动得分（四舍五入取整后总分，用于入库）
	AutoScore int
	// PerQuestion 每题得分明细：questionId -> 实际得分（可能是小数）
	// 用于前端展示"这道题你得了 X/Y 分"。
	PerQuestion map[int64]float64
	// UnansweredIDs 学生没作答的客观题 id
	UnansweredIDs []int64
	// WrongIDs 判错的客观题 id（含多选错选/漏选得分过低的）
	WrongIDs []int64
}

// ScoreExam 客观题自动判分纯函数。
// 抽出来是为了：1) 可单测；2) 后续 AI 模块做主观题初评时复用。
//
// 判分规则：
//   - single：学生答案 == 标准答案 得满分
//   - judge：同上
//   - multiple：部分给分
//   - 学生选了不在正确答案集合里的选项（错选）→ 本题 0 分
//   - 否则按"答对数量 / 正确答案总数" × 满分 给分
//     例如 5 个选项、正确答案 A,C,D 共 3 个：
//     学生选 A,C,D → 3/3 × 5 = 5 分（满分）
//     学生选 A,C   → 2/3 × 5 ≈ 3.33 分
//     学生选 A,B   → 错选 B → 0 分
//   - essay/fill/fileupload：跳过（老师手批）
//   - 学生没作答：不计分，记到 UnansweredIDs
func ScoreExam(questions []*model.ExamQuestionRow, answers map[string]string) ScoreResult {
	res := ScoreResult{PerQuestion: map[int64]float64{}}
	var totalRaw float64

	for _, q := range questions {
		// 主观题/填空/文件上传：老师手批，不自动判
		if q.Type == "essay" || q.Type == "fill" || q.Type == "fileupload" {
			continue
		}
		pick, ok := answers[strconv.FormatInt(q.Id, 10)]
		if !ok || strings.TrimSpace(pick) == "" {
			res.UnansweredIDs = append(res.UnansweredIDs, q.Id)
			continue
		}

		var got float64
		switch q.Type {
		case "multiple":
			got = scoreMultipleChoice(pick, q.Answer, float64(q.Score))
		case "judge":
			// 判断题单独走归一化比较：历史数据里"对/错"存成过 A/B、true/false、对/错，
			// 新版出题器统一存 true/false、学生端也提交 true/false。
			// 不归一化的话，老题库的判断题会永远判错（学生选对也是 0 分）。
			if normalizeJudge(pick) == normalizeJudge(q.Answer) {
				got = float64(q.Score)
			}
		default: // single
			if strings.EqualFold(strings.TrimSpace(pick), strings.TrimSpace(q.Answer)) {
				got = float64(q.Score)
			}
		}

		res.PerQuestion[q.Id] = got
		totalRaw += got
		if got < float64(q.Score) {
			res.WrongIDs = append(res.WrongIDs, q.Id)
		}
	}

	res.AutoScore = int(math.Round(totalRaw))
	return res
}

// normalizeJudge 判断题答案归一化。
//
// 为什么需要：判断题在不同时期被存成过三种写法——A/B（早期字母）、
// true/false（新版出题器）、对/错（人工录入）。判分时统一成 "true"/"false"，
// 否则"老题目的正确答案是 A、学生端提交 true"会永远判错。
func normalizeJudge(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "a", "true", "t", "1", "对", "√", "yes", "y", "正确":
		return "true"
	case "b", "false", "f", "0", "错", "×", "x", "no", "n", "错误":
		return "false"
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// scoreMultipleChoice 多选题按权重给分。
// 规则见 ScoreExam 注释。返回该题实际得分（0 ~ qScore）。
func scoreMultipleChoice(pick, answer string, qScore float64) float64 {
	pSet := splitChoice(pick)
	aSet := splitChoice(answer)
	if len(aSet) == 0 {
		return 0
	}

	pSetMap := make(map[string]bool, len(pSet))
	for _, s := range pSet {
		pSetMap[strings.ToUpper(s)] = true
	}
	aSetMap := make(map[string]bool, len(aSet))
	for _, s := range aSet {
		aSetMap[strings.ToUpper(s)] = true
	}

	// 错选：学生选了正确答案集合之外的选项 → 0 分
	for _, s := range pSet {
		if !aSetMap[strings.ToUpper(s)] {
			return 0
		}
	}

	// 漏选：统计命中数，按命中比例给分
	hit := 0
	for _, s := range aSet {
		if pSetMap[strings.ToUpper(s)] {
			hit++
		}
	}
	return qScore * float64(hit) / float64(len(aSet))
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
	sort.Strings(out)
	return out
}
