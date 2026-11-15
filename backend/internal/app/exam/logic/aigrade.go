package logic

// 考试主观题 AI 初评：复用作业的整条 AI 批改链路。
//
// 与作业侧（class/logic/aigradelogic.go）的差异只有两处：
//  1. 取数来源：exam_submissions / exam_questions，而不是作业表；
//  2. 建议表 assignment_ai_grades 的 assignment_id 固定写 0，
//     作为「这条记录属于考试提交」的标记 —— 回调入口据此分发
//     （两张提交表主键可能撞号，不能用 sid 裸查）。
//
// 其余完全一致：载荷由 Go 从库里组装、先落 processing 再入队、
// 结果只写建议表绝不碰成绩册、老师点「采纳」才落分。
import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"learningplatform/internal/app/class"
	classlogic "learningplatform/internal/app/class/logic"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/aiservice"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExamAiGradeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExamAiGradeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExamAiGradeLogic {
	return &ExamAiGradeLogic{ctx: ctx, svcCtx: svcCtx}
}

// loadSubmission 提交 + 考试 + 老师权限一起校验（Trigger / Get 共用）。
func (l *ExamAiGradeLogic) loadSubmission(sid int64, cu *class.CtxUser) (*model.ExamSubmissionRow, *model.ExamRow, *class.BizError) {
	sub, err := l.svcCtx.ExamStore.GetSubmission(sid)
	if err != nil || sub == nil {
		return nil, nil, &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(sub.ExamId)
	if err != nil || e == nil {
		return nil, nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, nil, be
	}
	return sub, e, nil
}

// Trigger 为一份考试提交发起 AI 初评（老师端按钮）。
//
// 与作业侧 Trigger 相同的顺序：先落 processing 记录再入队，
// 反过来 worker 的回调会先到，把刚落的结果覆盖回 processing。
func (l *ExamAiGradeLogic) Trigger(sid int64, cu *class.CtxUser) (*class.AiGradeTriggerResp, *class.BizError) {
	sub, _, be := l.loadSubmission(sid, cu)
	if be != nil {
		return nil, be
	}
	if !l.svcCtx.AI.Enabled() {
		return nil, &class.BizError{Code: http.StatusServiceUnavailable, Msg: "AI 功能未启用，请联系管理员配置 AiService"}
	}

	questions, err := l.svcCtx.ExamStore.ListQuestions(sub.ExamId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "读取考试题目失败"}
	}
	items := BuildExamGradeItems(questions, classlogic.AiGradeParseAnswers(sub.Answers))
	essayCount := ExamEssayQuestions(questions)
	if len(items) == 0 {
		msg := "这份提交没有可批改的主观题作答"
		if len(essayCount) > 0 {
			msg += "（学生未作答的题 AI 不打分）"
		}
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: msg}
	}

	taskId := classlogic.AiGradeNewTaskId()
	row := &model.AIGradeRow{
		SubmissionId: sub.Id,
		// assignment_id = 0 是「考试提交」的标记：作业 id 从 1 开始，永不冲突
		AssignmentId: 0,
		StudentId:    sub.UserId,
		TaskId:       taskId,
		Status:       model.AIGradeProcessing,
		MaxScore:     classlogic.AiGradeSumItemMaxScore(items),
		RequestedBy:  cu.Id,
	}
	if err := l.svcCtx.AIGradeStore.Upsert(row); err != nil {
		logx.Errorf("写入考试 AI 初评任务失败 submission=%d: %v", sub.Id, err)
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "发起批改失败，请稍后重试"}
	}

	receipt, err := l.svcCtx.AI.EnqueueGrade(l.ctx, aiservice.Actor{UserId: cu.Id, Role: cu.Role}, &aiservice.GradeRequest{
		SubmitId:  sub.Id,
		StudentId: sub.UserId,
		Items:     items,
		TaskId:    taskId,
	})
	if err != nil {
		// 入队没成功，任务永远不会回调：立刻标 failed，别让界面挂着转圈
		if uerr := l.svcCtx.AIGradeStore.UpdateStatus(sub.Id, model.AIGradeFailed, err.Error()); uerr != nil {
			logx.Errorf("标记考试 AI 批改失败状态出错 submission=%d: %v", sub.Id, uerr)
		}
		return nil, classlogic.AiGradeError(err)
	}

	grade := classlogic.AiGradeViewOf(row, len(essayCount), true)
	grade.TaskId = receipt.TaskId
	return &class.AiGradeTriggerResp{
		Message: "已提交 AI 初评，出结果后自动刷新",
		Grade:   grade,
	}, nil
}

// Get 读某份考试提交的 AI 初评（processing 卡住时对账一次）。
func (l *ExamAiGradeLogic) Get(sid int64, cu *class.CtxUser) (*class.AiGradeView, *class.BizError) {
	_, e, be := l.loadSubmission(sid, cu)
	if be != nil {
		return nil, be
	}
	essayCount := 0
	if questions, qerr := l.svcCtx.ExamStore.ListQuestions(e.Id); qerr == nil {
		essayCount = len(ExamEssayQuestions(questions))
	}

	row, err := l.svcCtx.AIGradeStore.GetBySubmission(sid)
	if errors.Is(err, model.ErrNotFound) {
		return classlogic.AiGradeViewOf(nil, essayCount, l.svcCtx.AI.Enabled()), nil
	}
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "读取批改记录失败"}
	}

	if row.Status == model.AIGradeProcessing && row.TaskId != "" && l.svcCtx.AI.Enabled() {
		classlogic.AiGradeReconcileRow(l.ctx, l.svcCtx, row, aiservice.Actor{UserId: cu.Id, Role: cu.Role})
	}
	return classlogic.AiGradeViewOf(row, essayCount, l.svcCtx.AI.Enabled()), nil
}

// ExamEssayQuestions 挑出考试里需要 AI 批改的主观题（essay）。
// 填空有标准答案（但考试判分把它划给老师手批）、文件上传 AI 读不了，都不送 AI。
func ExamEssayQuestions(questions []*model.ExamQuestionRow) []*model.ExamQuestionRow {
	out := make([]*model.ExamQuestionRow, 0, len(questions))
	for _, q := range questions {
		if q != nil && q.Type == "essay" {
			out = append(out, q)
		}
	}
	return out
}

// BuildExamGradeItems 组装送批改的载荷（纯函数，便于单测）。
//
// 与作业侧 BuildGradeItems 同一套原则：
//   - 只送 essay 题型；
//   - 学生没作答的题跳过（AI 打 0 分没有信息量，白烧额度）；
//   - 分值缺省兜底 100；
//   - 上限 30 题（与 ai-service 的 MAX_ITEMS 一致，那边超了会静默截断）。
func BuildExamGradeItems(questions []*model.ExamQuestionRow, answers map[string]string) []aiservice.GradeItem {
	essays := ExamEssayQuestions(questions)
	items := make([]aiservice.GradeItem, 0, len(essays))
	for _, q := range essays {
		text := answers[strconv.FormatInt(q.Id, 10)]
		if text == "" {
			continue
		}
		maxScore := q.Score
		if maxScore <= 0 {
			maxScore = class.DefaultAiGradeMaxScore
		}
		items = append(items, aiservice.GradeItem{
			QuestionId:     q.Id,
			Stem:           q.Stem,
			StudentAnswer:  text,
			StandardAnswer: q.Answer,
			MaxScore:       maxScore,
		})
	}
	if len(items) > class.MaxAiGradeItems {
		items = items[:class.MaxAiGradeItems]
	}
	return items
}
