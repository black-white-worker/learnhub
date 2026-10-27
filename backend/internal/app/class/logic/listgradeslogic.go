package logic

// 班级成绩册（对标 Canvas gradebook）。
//
// 口径（改造后）：
//   作业得分率 = Σ作业得分 / Σ作业满分 × 100
//   考试得分率 = Σ考试得分 / Σ考试满分 × 100
//   签到得分率 = 已签次数 / 班级签到场次 × 100
//   视频得分率 = 已看完视频数 / 班级绑定课程视频总数 × 100
//   加权总分   = Σ(各类得分率 × 该类权重) / Σ权重
//
// 改造前的问题：视频项直接写死 `100 * 视频权重`（人人满分），
// 签到次数×10 不封顶，考试权重被丢弃 —— 总分没有任何教学含义。
// 现在每一项都来自真实数据，各自封顶 100，空的分类不占权重。

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"learningplatform/internal/app/class"
	"learningplatform/internal/gradebook"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"
)

type ListGradesLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGradesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGradesLogic {
	return &ListGradesLogic{ctx: ctx, svcCtx: svcCtx}
}

// ListGrades 返回整个班级的成绩册（仅班级老师 / 管理员可见）。
func (l *ListGradesLogic) ListGrades(classId int64, cu *class.CtxUser) (*class.ListGradesResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	rows, weights, err := l.buildRows(classId)
	if err != nil {
		return nil, err
	}
	return &class.ListGradesResp{
		Rule: model.RuleMap(weights.Video, weights.Assignment, weights.Signin, weights.Exam),
		Rows: rows,
	}, nil
}

// buildRows 取数 + 计算，返回按总分降序排好的行与生效权重。
func (l *ListGradesLogic) buildRows(classId int64) ([]*model.GradeRow, gradebook.Weights, error) {
	raw, err := l.svcCtx.ClassStore.ListGradebookRaw(classId)
	if err != nil {
		return nil, gradebook.Weights{}, err
	}
	vW, aW, sW, eW, _ := l.svcCtx.ClassStore.GetGradeRule(classId)
	base := gradebook.Weights{Video: vW, Assignment: aW, Signin: sW, Exam: eW}

	// 班级里是否真的存在某一类考核：决定该类是否占权重。
	// 同一班级各行的分母相同，遍历取"存在"即可。
	hasAssign, hasExam, hasSignin, hasVideo := false, false, false, false
	for _, r := range raw {
		hasAssign = hasAssign || r.AssignMax > 0
		hasExam = hasExam || r.ExamMax > 0
		hasSignin = hasSignin || r.SigninTotal > 0
		hasVideo = hasVideo || r.VideoTotal > 0
	}
	weights := gradebook.EffectiveWeights(base, hasVideo, hasAssign, hasSignin, hasExam)

	out := make([]*model.GradeRow, 0, len(raw))
	for _, r := range raw {
		g := r.ToGradeRow()
		g.AssignmentPercent = gradebook.Percent(r.AssignScore, r.AssignMax)
		g.ExamPercent = gradebook.Percent(r.ExamScore, r.ExamMax)
		g.SigninPercent = gradebook.Ratio(r.SigninCnt, r.SigninTotal)
		g.VideoPercent = gradebook.Ratio(r.VideoDone, r.VideoTotal)
		g.TotalScore = gradebook.Total(map[string]float64{
			"video":      g.VideoPercent,
			"assignment": g.AssignmentPercent,
			"signin":     g.SigninPercent,
			"exam":       g.ExamPercent,
		}, weights)
		g.Letter = gradebook.Letter(g.TotalScore)
		g.AvgScore = g.AssignmentPercent // 兼容旧的"作业平均分"列
		out = append(out, g)
	}

	// 成绩册默认按总分降序（老师最关心排名）
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].TotalScore == out[j].TotalScore {
			return out[i].Username < out[j].Username
		}
		return out[i].TotalScore > out[j].TotalScore
	})
	return out, weights, nil
}

// ExportCSV 导出成绩单（UTF-8 BOM + CSV，Excel 直接打开不乱码）。
func (l *ListGradesLogic) ExportCSV(classId int64, cu *class.CtxUser) ([]byte, string, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, "", be
	}
	cls, err := l.svcCtx.ClassStore.GetClass(classId)
	if err != nil {
		return nil, "", &class.BizError{Code: 404, Msg: "班级不存在"}
	}
	rows, weights, err := l.buildRows(classId)
	if err != nil {
		return nil, "", err
	}

	var b strings.Builder
	// BOM：让 Excel 按 UTF-8 解析中文
	b.WriteString("\ufeff")
	b.WriteString("学生,学号(id),作业得分率(%),作业已交,考试得分率(%),签到次数,签到率(%),视频已看,视频总数,视频率(%),加权总分,等级\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("%s,%d,%.1f,%d,%.1f,%d,%.1f,%d,%d,%.1f,%.1f,%s\n",
			csvEscape(r.Username), r.UserId,
			r.AssignmentPercent, r.AssignmentCnt,
			r.ExamPercent,
			r.SigninCnt, r.SigninPercent,
			r.VideoDone, r.VideoTotal, r.VideoPercent,
			r.TotalScore, r.Letter,
		))
	}
	// 附上本次使用的权重，避免"导出后权重变了没法解释"
	b.WriteString(fmt.Sprintf("\n权重(视频/作业/签到/考试),%d/%d/%d/%d\n",
		weights.Video, weights.Assignment, weights.Signin, weights.Exam))

	filename := fmt.Sprintf("%s-成绩单.csv", cls.Name)
	return []byte(b.String()), filename, nil
}

// csvEscape 处理逗号/引号/换行，避免把 CSV 撑破。
func csvEscape(s string) string {
	if !strings.ContainsAny(s, ",\"\n\r") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
