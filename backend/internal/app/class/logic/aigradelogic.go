package logic

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/aiservice"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// AiGradeLogic 主观题 AI 初评：发起、回调落库、查询（带对账）。
//
// 三条链路的共同原则：**AI 的结果只写 assignment_ai_grades，绝不碰成绩册**。
// 老师看完点「采纳」走原来的 /api/assignment/submission/:sid/grade，
// 那条路才有 RequireTeacher + 成绩册写入。
type AiGradeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewAiGradeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *AiGradeLogic {
	return &AiGradeLogic{ctx: ctx, svcCtx: svcCtx}
}

// ---- 内部工具 ----

// loadSubmission 提交 + 作业 + 老师权限三件事一起校验，三个接口都要用。
func (l *AiGradeLogic) loadSubmission(sid int64, cu *class.CtxUser) (*model.SubmissionRow, *model.AssignmentRow, *class.BizError) {
	sub, err := l.svcCtx.ClassStore.GetSubmission(sid)
	if err != nil || sub == nil {
		return nil, nil, &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	a, err := l.svcCtx.ClassStore.GetAssignment(sub.AssignmentId)
	if err != nil || a == nil {
		return nil, nil, &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, a.ClassId, cu); be != nil {
		return nil, nil, be
	}
	return sub, a, nil
}

// requireEnabled 把「没配 AI」和「AI 挂了」分开：前者是运维状态，配好就有；
// 后者是运行时故障，重试可能好。文案不一样，老师不用白等。
func (l *AiGradeLogic) requireEnabled() *class.BizError {
	if !l.svcCtx.AI.Enabled() {
		return &class.BizError{Code: http.StatusServiceUnavailable, Msg: "AI 功能未启用，请联系管理员配置 AiService"}
	}
	return nil
}

// newTaskId 生成 32 位 hex 任务号（和 ai-service 的 uuid4().hex 同格式）。
// 用它拼 Redis key，所以只允许 hex 字符。
func newTaskId() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand 失败基本等于系统熵源坏了，退化成时间戳也能保证本次进程内唯一
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000")))[:32]
	}
	return hex.EncodeToString(b)
}

func parseAnswersMap(raw string) map[string]string {
	if raw == "" {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]string{}
	}
	if m == nil {
		m = map[string]string{}
	}
	return m
}

func sumItemMaxScore(items []aiservice.GradeItem) int {
	total := 0
	for _, it := range items {
		total += it.MaxScore
	}
	return total
}

// decodeItems 把落库的逐题 JSON 还原成结构；坏数据返回空切片而不是报错，
// 老师至少还能看到总分和总评。
func decodeItems(raw string) []class.AiGradeItemResult {
	if raw == "" {
		return []class.AiGradeItemResult{}
	}
	var out []class.AiGradeItemResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		logx.Errorf("AI 初评逐题结果解析失败: %v", err)
		return []class.AiGradeItemResult{}
	}
	if out == nil {
		out = []class.AiGradeItemResult{}
	}
	return out
}

// gradeView 落库行 → 接口视图。essayCount 单独传：没发起过批改时也要告诉前端有几道主观题。
func gradeView(row *model.AIGradeRow, essayCount int, enabled bool) *class.AiGradeView {
	if row == nil {
		return &class.AiGradeView{
			Status:     "",
			Items:      []class.AiGradeItemResult{},
			Enabled:    enabled,
			EssayCount: essayCount,
		}
	}
	v := &class.AiGradeView{
		SubmissionId: row.SubmissionId,
		TaskId:       row.TaskId,
		Status:       row.Status,
		Score:        row.Score,
		MaxScore:     row.MaxScore,
		Comment:      row.Comment,
		Error:        row.ErrorText,
		Items:        decodeItems(row.Items),
		Enabled:      enabled,
		EssayCount:   essayCount,
	}
	if row.UpdatedAt.Valid {
		v.UpdatedAt = row.UpdatedAt.Time.Format("2006-01-02 15:04")
	}
	return v
}

// aiGradeError 把客户端错误翻译成给老师看的话术。
// ErrDisabled/ErrUnavailable 是「等一等或找管理员」，4xx 是「这次请求本身不对」。
func aiGradeError(err error) *class.BizError {
	switch {
	case errors.Is(err, aiservice.ErrDisabled):
		return &class.BizError{Code: http.StatusServiceUnavailable, Msg: "AI 功能未启用，请联系管理员配置 AiService"}
	case errors.Is(err, aiservice.ErrUnavailable):
		return &class.BizError{Code: http.StatusServiceUnavailable, Msg: "AI 服务暂时不可用，请稍后重试"}
	case errors.Is(err, aiservice.ErrTaskGone):
		return &class.BizError{Code: http.StatusNotFound, Msg: "批改任务已过期，请重新发起"}
	default:
		return &class.BizError{Code: http.StatusBadGateway, Msg: err.Error()}
	}
}

// AiGradeError aiGradeError 的导出封装（考试模块入队失败时复用同一套话术）。
func AiGradeError(err error) *class.BizError {
	return aiGradeError(err)
}

// ---- 发起 ----

// Trigger 为一份提交发起 AI 初评（老师端按钮）。
//
// 顺序很重要：先落一条 processing，再入队。
// 反过来写的话，批改快的时候 worker 的回调会先到，Go 后写的 processing
// 会把已经拿到的结果覆盖掉，界面永远停在「批改中」。
func (l *AiGradeLogic) Trigger(sid int64, cu *class.CtxUser) (*class.AiGradeTriggerResp, *class.BizError) {
	sub, a, be := l.loadSubmission(sid, cu)
	if be != nil {
		return nil, be
	}
	if be := l.requireEnabled(); be != nil {
		return nil, be
	}

	questions, err := l.svcCtx.ClassStore.ListQuestions(a.Id)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "读取作业题目失败"}
	}
	items := class.BuildGradeItems(questions, parseAnswersMap(sub.Answers), sub.Content)
	essayCount := len(class.EssayQuestions(questions))
	if len(items) == 0 {
		msg := "这份提交没有可批改的主观题作答"
		if essayCount > 0 {
			msg += "（学生未作答的题 AI 不打分）"
		}
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: msg}
	}

	taskId := newTaskId()
	row := &model.AIGradeRow{
		SubmissionId: sub.Id,
		AssignmentId: a.Id,
		CourseId:     a.CourseId,
		StudentId:    sub.UserId,
		TaskId:       taskId,
		Status:       model.AIGradeProcessing,
		MaxScore:     sumItemMaxScore(items),
		RequestedBy:  cu.Id,
	}
	if err := l.svcCtx.AIGradeStore.Upsert(row); err != nil {
		logx.Errorf("写入 AI 初评任务失败 submission=%d: %v", sub.Id, err)
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "发起批改失败，请稍后重试"}
	}

	receipt, err := l.svcCtx.AI.EnqueueGrade(l.ctx, aiservice.Actor{UserId: cu.Id, Role: cu.Role}, &aiservice.GradeRequest{
		SubmitId:     sub.Id,
		AssignmentId: a.Id,
		CourseId:     a.CourseId,
		StudentId:    sub.UserId,
		Items:        items,
		TaskId:       taskId,
	})
	if err != nil {
		// 入队没成功，任务永远不会回调：立刻标 failed，别让界面挂着转圈
		if uerr := l.svcCtx.AIGradeStore.UpdateStatus(sub.Id, model.AIGradeFailed, err.Error()); uerr != nil {
			logx.Errorf("标记 AI 批改失败状态出错 submission=%d: %v", sub.Id, uerr)
		}
		return nil, aiGradeError(err)
	}

	grade := gradeView(row, essayCount, true)
	grade.TaskId = receipt.TaskId
	return &class.AiGradeTriggerResp{
		Message: "已提交 AI 初评，出结果后自动刷新",
		Grade:   grade,
	}, nil
}

// ---- 回调 ----

// Callback ai-service worker 写回批改结果（服务间调用，鉴权走 X-Internal-Token）。
//
// 一份批改任务可能来自作业提交，也可能来自考试提交（考试复用作业的整条
// AI 批改链路，见 exam/logic/aigrade.go）。worker 统一回这个入口，这里按
// 建议表的 assignment_id 分发：>0 是作业，=0 是考试。
//
// 优先按 taskId 反查记录归属：作业提交表和考试提交表是两张独立表，主键可能
// 撞号（作业提交 5 和考试提交 5 同时存在）。只按 sid 查表会把考试结果写到
// 同号作业提交上——先查建议表就能精确路由。
func (l *AiGradeLogic) Callback(sid int64, req *class.AiGradeCallbackReq) *class.BizError {
	if req.TaskId != "" {
		if row, err := l.svcCtx.AIGradeStore.GetByTask(req.TaskId); err == nil && row != nil {
			if row.AssignmentId == 0 {
				return l.callbackExam(sid, req)
			}
			return l.callbackAssignment(sid, req, row)
		}
	}
	// 没有 taskId 或建议表记录丢失（数据库重建过）：按 sid 依次尝试两张表
	if sub, err := l.svcCtx.ClassStore.GetSubmission(sid); err == nil && sub != nil {
		return l.callbackAssignment(sid, req, nil)
	}
	if esub, err := l.svcCtx.ExamStore.GetSubmission(sid); err == nil && esub != nil {
		return l.callbackExam(sid, req)
	}
	return &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
}

// callbackAssignment 作业提交的回调：定位建议表行后套用结果并落库。
func (l *AiGradeLogic) callbackAssignment(sid int64, req *class.AiGradeCallbackReq, row *model.AIGradeRow) *class.BizError {
	sub, err := l.svcCtx.ClassStore.GetSubmission(sid)
	if err != nil || sub == nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	a, err := l.svcCtx.ClassStore.GetAssignment(sub.AssignmentId)
	if err != nil || a == nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "作业不存在"}
	}
	if row == nil {
		row, err = l.svcCtx.AIGradeStore.GetBySubmission(sid)
		if errors.Is(err, model.ErrNotFound) {
			// Go 这边没有记录（比如数据库重建过），按提交现场补一条，结果照样落库
			row = &model.AIGradeRow{SubmissionId: sub.Id, AssignmentId: a.Id, CourseId: a.CourseId, StudentId: sub.UserId}
		} else if err != nil {
			return &class.BizError{Code: http.StatusInternalServerError, Msg: "读取批改记录失败"}
		}
	}
	return l.finishCallback(sid, row, req)
}

// callbackExam 考试提交的回调：同一条建议表（assignment_id=0 标记考试来源）。
func (l *AiGradeLogic) callbackExam(sid int64, req *class.AiGradeCallbackReq) *class.BizError {
	sub, err := l.svcCtx.ExamStore.GetSubmission(sid)
	if err != nil || sub == nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	row, err := l.svcCtx.AIGradeStore.GetBySubmission(sid)
	if errors.Is(err, model.ErrNotFound) {
		row = &model.AIGradeRow{SubmissionId: sub.Id, AssignmentId: 0, StudentId: sub.UserId}
	} else if err != nil {
		return &class.BizError{Code: http.StatusInternalServerError, Msg: "读取批改记录失败"}
	}
	return l.finishCallback(sid, row, req)
}

// finishCallback taskId 一致性校验 + 结果套用 + 落库（作业/考试共用）。
//
// taskId 对不上就直接拒：那是上一轮批改的迟到回调，
// 收下来会把新任务的结果写成旧数据。
func (l *AiGradeLogic) finishCallback(sid int64, row *model.AIGradeRow, req *class.AiGradeCallbackReq) *class.BizError {
	if row.TaskId != "" && req.TaskId != "" && row.TaskId != req.TaskId {
		logx.Errorf("AI 批改回调 taskId 不匹配 submission=%d expect=%s got=%s", sid, row.TaskId, req.TaskId)
		return &class.BizError{Code: http.StatusConflict, Msg: "回调的任务和当前任务不一致，已忽略"}
	}

	applyCallback(row, req)
	if err := l.svcCtx.AIGradeStore.Upsert(row); err != nil {
		logx.Errorf("落库 AI 初评结果失败 submission=%d: %v", sid, err)
		return &class.BizError{Code: http.StatusInternalServerError, Msg: "批改结果保存失败"}
	}
	return nil
}

// applyCallback 把回调结果套到记录行上（纯内存操作，作业/考试共用）。
func applyCallback(row *model.AIGradeRow, req *class.AiGradeCallbackReq) {
	itemsJSON, merr := json.Marshal(req.Items)
	if merr != nil {
		itemsJSON = []byte("[]")
	}

	row.TaskId = firstNonEmpty(req.TaskId, row.TaskId)
	row.MaxScore = req.MaxScore
	if row.MaxScore <= 0 {
		row.MaxScore = sumCallbackMax(req.Items)
	}
	row.Score = req.Score
	// comment / error 是 varchar(1024) / varchar(512)，LLM 偶尔会写一大段，
	// 超长会让整条 UPSERT 在 strict 模式下报错，回调白跑一次重试。这里按列宽截断。
	row.Comment = truncateRunes(req.Comment, 1000)
	row.Items = string(itemsJSON)
	row.ErrorText = ""
	if !req.ParseOk {
		// AI 有题没批出来：分数一律不采纳，成绩保持空，转人工
		row.Status = model.AIGradeManual
		row.Score = 0
		row.ErrorText = "AI 未能给出全部题目的分数，请人工批改"
	} else {
		row.Status = model.AIGradeDone
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func sumCallbackMax(items []class.AiGradeItemResult) int {
	total := 0
	for _, it := range items {
		total += it.MaxScore
	}
	return total
}

// ---- 查询 + 对账 ----

// Get 读某份提交的 AI 初评。
//
// 卡在 processing 时会拿 taskId 去 ai-service 查一次真实状态再返回：
// 回调是 worker 发的，它可能因为 Go 重启 / 网络抖动丢掉，
// 没有这层对账的话界面上就永远转圈，老师只能重跑一遍（再烧一次额度）。
func (l *AiGradeLogic) Get(sid int64, cu *class.CtxUser) (*class.AiGradeView, *class.BizError) {
	// loadSubmission 顺带把「这份提交存在、属于这个班、当前用户是班内老师」校验掉了
	_, a, be := l.loadSubmission(sid, cu)
	if be != nil {
		return nil, be
	}
	essayCount := 0
	if questions, err := l.svcCtx.ClassStore.ListQuestions(a.Id); err == nil {
		essayCount = len(class.EssayQuestions(questions))
	}

	row, err := l.svcCtx.AIGradeStore.GetBySubmission(sid)
	if errors.Is(err, model.ErrNotFound) {
		return gradeView(nil, essayCount, l.svcCtx.AI.Enabled()), nil
	}
	if err != nil {
		return nil, &class.BizError{Code: http.StatusInternalServerError, Msg: "读取批改记录失败"}
	}

	if row.Status == model.AIGradeProcessing && row.TaskId != "" && l.svcCtx.AI.Enabled() {
		l.reconcile(row, cu)
	}
	return gradeView(row, essayCount, l.svcCtx.AI.Enabled()), nil
}

// reconcile 用 ai-service 的任务状态修正本地行。只改 row，落库交给调用后的 Upsert。
func (l *AiGradeLogic) reconcile(row *model.AIGradeRow, cu *class.CtxUser) {
	reconcileRow(l.ctx, l.svcCtx, row, aiservice.Actor{UserId: cu.Id, Role: cu.Role})
}

// reconcileRow 对账的实现体（独立出来供考试模块复用）。
func reconcileRow(ctx context.Context, svcCtx *svc.ServiceContext, row *model.AIGradeRow, actor aiservice.Actor) {
	st, err := svcCtx.AI.GetGradeTask(ctx, actor, row.TaskId)
	if err != nil {
		if errors.Is(err, aiservice.ErrTaskGone) {
			row.Status = model.AIGradeFailed
			row.ErrorText = "批改任务记录已过期（超过 7 天），请重新发起"
			persistReconcile(svcCtx, row)
		}
		// 其他错误（服务不可用）不动状态：下次轮询还会再试
		return
	}
	switch st.Status {
	case model.AIGradeDone, model.AIGradeManual:
		if st.Result != nil {
			itemsJSON, merr := json.Marshal(toViewItems(st.Result.Items))
			if merr == nil {
				row.Items = string(itemsJSON)
			}
			row.Comment = truncateRunes(st.Result.Comment, 1000)
			row.MaxScore = st.Result.MaxScore
			row.Score = st.Result.Score
		}
		if st.Status == model.AIGradeManual || (st.Result != nil && !st.Result.ParseOk) {
			row.Status = model.AIGradeManual
			row.Score = 0
			row.ErrorText = "AI 未能给出全部题目的分数，请人工批改"
		} else {
			row.Status = model.AIGradeDone
			row.ErrorText = ""
		}
		persistReconcile(svcCtx, row)
	case model.AIGradeFailed:
		row.Status = model.AIGradeFailed
		row.ErrorText = truncateRunes(firstNonEmpty(st.Error, "AI 批改失败"), 500)
		persistReconcile(svcCtx, row)
	}
}

func persistReconcile(svcCtx *svc.ServiceContext, row *model.AIGradeRow) {
	if err := svcCtx.AIGradeStore.Upsert(row); err != nil {
		logx.Errorf("对账修正 AI 初评状态失败 submission=%d: %v", row.SubmissionId, err)
	}
}

// toViewItems aiservice 的结果结构 → 前端结构（两者字段一样，只是来源包不同）。
func toViewItems(items []aiservice.GradeItemResult) []class.AiGradeItemResult {
	out := make([]class.AiGradeItemResult, 0, len(items))
	for _, it := range items {
		one := class.AiGradeItemResult{
			QuestionId:  it.QuestionId,
			MaxScore:    it.MaxScore,
			Comment:     it.Comment,
			ErrorPoints: it.ErrorPoints,
			Suggestion:  it.Suggestion,
			ParseOk:     it.ParseOk,
		}
		score := it.Score
		one.Score = &score
		out = append(out, one)
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// ---- 批阅队列用的批量读取 ----

// SummariesFor 批量取多份提交的 AI 初评摘要（SpeedGrader 一次拉一叠，逐条查会 N+1）。
// 没发起过批改的提交不出现在 map 里，前端按 key 取不到就当「未发起」。
// 读失败返回空 map：AI 是锦上添花，不能因为它把批阅页打不开。
func (l *AiGradeLogic) SummariesFor(submissionIds []int64) map[int64]*class.AiGradeView {
	out := map[int64]*class.AiGradeView{}
	if len(submissionIds) == 0 {
		return out
	}
	rows, err := l.svcCtx.AIGradeStore.ListBySubmissions(submissionIds)
	if err != nil {
		logx.Errorf("批量读取 AI 初评失败: %v", err)
		return out
	}
	for sid, row := range rows {
		out[sid] = gradeView(row, 0, l.svcCtx.AI.Enabled())
	}
	return out
}

// ===== 供考试模块复用的导出封装 =====
//
// 考试主观题复用作业的整条 AI 批改链路（同一张建议表、同一个 ai-service 队列、
// 同一个回调入口）。这里把作业侧沉淀的工具函数按导出封装开放出去，
// 考试的 logic 只做「取数 + 授权」的差异部分，不复制粘贴。

// AiGradeParseAnswers 解析学生作答 JSON（坏数据返回空 map）。
func AiGradeParseAnswers(raw string) map[string]string {
	return parseAnswersMap(raw)
}

// AiGradeViewOf 落库行 → 前端视图。
func AiGradeViewOf(row *model.AIGradeRow, essayCount int, enabled bool) *class.AiGradeView {
	return gradeView(row, essayCount, enabled)
}

// AiGradeSumItemMaxScore 批改载荷的满分合计（写建议表 max_score 用）。
func AiGradeSumItemMaxScore(items []aiservice.GradeItem) int {
	return sumItemMaxScore(items)
}

// AiGradeNewTaskId 生成 32 位 hex 任务号（拼 Redis key，只允许 hex 字符）。
func AiGradeNewTaskId() string {
	return newTaskId()
}

// AiGradeReconcileRow processing 卡住时拿 taskId 去 ai-service 对账一次。
// 回调是 worker 发的，可能因为重启 / 网络抖动丢掉；没有这层对账界面就永远转圈。
func AiGradeReconcileRow(ctx context.Context, svcCtx *svc.ServiceContext, row *model.AIGradeRow, actor aiservice.Actor) {
	reconcileRow(ctx, svcCtx, row, actor)
}
