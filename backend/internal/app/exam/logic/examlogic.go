package logic

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"learningplatform/internal/app/class"
	"learningplatform/internal/app/exam"
	"learningplatform/internal/app/exam/logic/examrules"
	"learningplatform/internal/authz"
	"learningplatform/internal/model"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

type ExamLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewExamLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ExamLogic {
	return &ExamLogic{ctx: ctx, svcCtx: svcCtx}
}

// CreateExam 老师创建考试
func (l *ExamLogic) CreateExam(classId int64, req *exam.CreateExamReq, cu *class.CtxUser) (*exam.CreateExamResp, error) {
	if be := class.RequireTeacher(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	if req.Title == "" {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "考试标题不能为空"}
	}
	start, err1 := time.ParseInLocation("2006-01-02T15:04", req.StartAt, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02T15:04", req.EndAt, time.Local)
	if err1 != nil || err2 != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "开始/结束时间格式应为 YYYY-MM-DDTHH:mm"}
	}
	if !end.After(start) {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "结束时间必须晚于开始时间"}
	}
	dur := req.DurationMin
	if dur <= 0 {
		dur = 60
	}
	total := req.TotalScore
	if total <= 0 {
		total = 100
	}
	id, err := l.svcCtx.ExamStore.InsertExam(&model.ExamRow{
		ClassId: classId, Title: req.Title, Description: req.Description,
		StartAt: start, EndAt: end, DurationMin: dur, TotalScore: total, CreatedBy: cu.Id,
	})
	if err != nil {
		return nil, err
	}
	// 发布考试时同步广播到消息中心（与公告/作业一致）
	_ = l.svcCtx.ClassStore.BroadcastNotification(
		classId, cu.Id, "exam",
		"新考试："+req.Title,
		"开考时间 "+start.Format("01-02 15:04")+"，时长 "+strconv.Itoa(dur)+" 分钟",
		"/exam/"+strconv.FormatInt(id, 10),
	)
	return &exam.CreateExamResp{Id: id}, nil
}

// ListExams 班级考试列表（分页）
func (l *ExamLogic) ListExams(classId int64, page, size int, cu *class.CtxUser) (interface{}, error) {
	if _, be := class.RequireClassMember(l.svcCtx, classId, cu); be != nil {
		return nil, be
	}
	list, total, err := l.svcCtx.ExamStore.ListExamsByClassPaged(classId, page, size)
	if err != nil {
		return nil, err
	}
	return &exam.PagedResp{List: list, Total: total, Page: page, PageSize: size}, nil
}

// Detail 考试详情：学生看题目+自己提交，老师看提交列表
func (l *ExamLogic) Detail(examId int64, cu *class.CtxUser) (interface{}, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	questions, _ := l.svcCtx.ExamStore.ListQuestions(examId)

	// 学生：返回自己的提交；老师：返回所有人提交
	if role == "teacher" || role == "admin" {
		subs, _ := l.svcCtx.ExamStore.ListSubmissions(examId)

		// 老师端要"逐份看答卷 + 每题得了多少分"，所以按 submission 逐个算一遍客观题得分。
		// 改造前这里只返回题目和提交列表，前端弹窗拿不到每题分，
		// 只能去读学生的 detail.perQuestion（老师端根本没有这个字段）→ 每道题都显示 0 分。
		scores := buildSubmissionScores(subs, questions)

		return map[string]interface{}{
			"exam":             e,
			"questions":        questions,
			"submissions":      subs,
			"submissionScores": scores,
			"myRole":           role,
		}, nil
	}

	sub, _ := l.svcCtx.ExamStore.GetMySubmission(examId, cu.Id)
	now := time.Now()
	w := windowOf(e)

	// 答案可见性：只有"整场考试结束"后才下发给学生。
	// 改造前是"交卷即给答案"，考试期间先交卷的人可以把答案传给还在考的同学。
	// perQuestion（每题得分明细）同样受这个开关控制。
	answersVisible := w.RevealAnswers(now)
	hasSubmitted := sub != nil && sub.Status != "" && sub.Status != model.SubmitStatusStarted
	var perQuestion map[int64]float64
	if hasSubmitted && answersVisible {
		var answers map[string]string
		if sub.Answers != "" {
			_ = json.Unmarshal([]byte(sub.Answers), &answers)
		}
		if answers == nil {
			answers = map[string]string{}
		}
		scoreRes := ScoreExam(questions, answers)
		perQuestion = scoreRes.PerQuestion
	}
	if !answersVisible {
		// 未结束（或已结束但学生还没交卷）：一律清空标准答案
		for _, q := range questions {
			q.Answer = ""
		}
	}

	// 个人交卷截止时间：以学生真实开考时间为起点，前端只展示不计算
	var deadlineAt *string
	if started := startedAtOf(sub); !started.IsZero() {
		d := w.Deadline(started).Format(time.RFC3339)
		deadlineAt = &d
	}
	// 还能不能交：已交卷 → false；时限内 → true
	canSubmit := sub == nil || sub.Status == model.SubmitStatusStarted
	if canSubmit {
		canSubmit = w.CheckSubmit(now, startedAtOf(sub)) == nil
	}

	return map[string]interface{}{
		"exam":           e,
		"questions":      questions,
		"mySubmission":   sub,
		"perQuestion":    perQuestion,
		"myRole":         role,
		"serverTime":     now.Format(time.RFC3339),
		"deadlineAt":     deadlineAt,
		"canSubmit":      canSubmit,
		"answersVisible": answersVisible,
	}, nil
}

// buildSubmissionScores 按提交逐份计算客观题每题得分（老师端弹窗用）。
//
// 纯函数，便于回归测试：输入提交列表 + 题目列表，输出
// {"<submissionId>": {"<questionId>": 得分}}。
// 主观题（essay/fill/fileupload）不会出现在明细里，前端据此显示"待老师批"；
// 未作答的客观题也不出现，前端显示"未答"。
func buildSubmissionScores(subs []*model.ExamSubmissionRow, questions []*model.ExamQuestionRow) map[int64]map[int64]float64 {
	scores := make(map[int64]map[int64]float64, len(subs))
	for _, s := range subs {
		var ans map[string]string
		if s.Answers != "" {
			_ = json.Unmarshal([]byte(s.Answers), &ans)
		}
		if ans == nil {
			ans = map[string]string{}
		}
		scores[s.Id] = ScoreExam(questions, ans).PerQuestion
	}
	return scores
}

// startedAtOf 取提交记录里的开考时间（没有记录时返回零值）。
func startedAtOf(sub *model.ExamSubmissionRow) time.Time {
	if sub != nil && sub.StartedAt.Valid {
		return sub.StartedAt.Time
	}
	return time.Time{}
}

// AddQuestion 老师给考试加题（空题卡，点题型栏追加）。
// 对标腾讯问卷：题干可以先空着，后端只做最基本校验。
func (l *ExamLogic) AddQuestion(examId int64, req *exam.AddQuestionReq, cu *class.CtxUser) (*exam.AddQuestionResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, be
	}
	qt := req.Type
	if qt == "" {
		qt = "single"
	}
	// 允许的题型：single单选 multiple多选 judge判断 fill填空 essay简答 fileupload文件上传
	allowed := map[string]bool{"single": true, "multiple": true, "judge": true, "fill": true, "essay": true, "fileupload": true}
	if !allowed[qt] {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题型不支持"}
	}
	score := req.Score
	if score <= 0 {
		score = 5
	}
	// 新题追加到末尾
	maxOrder, _ := l.svcCtx.ExamStore.MaxOrderNum(examId)
	id, err := l.svcCtx.ExamStore.InsertQuestion(&model.ExamQuestionRow{
		ExamId: examId, Type: qt, Stem: req.Stem, Options: req.Options, Answer: req.Answer,
		Score: score, OrderNum: maxOrder + 1, Required: 1,
	})
	if err != nil {
		return nil, err
	}
	return &exam.AddQuestionResp{Id: id}, nil
}

// UpdateQuestion 编辑单题（题干/选项/答案/分值/必答）。
func (l *ExamLogic) UpdateQuestion(qid int64, req *exam.UpdateQuestionReq, cu *class.CtxUser) error {
	q, err := l.svcCtx.ExamStore.GetQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(q.ExamId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	required := q.Required
	if req.Required != nil {
		required = *req.Required
	}
	score := req.Score
	if score <= 0 {
		score = q.Score
	}
	qt := req.Type
	if qt == "" {
		qt = q.Type
	}
	return l.svcCtx.ExamStore.UpdateQuestion(&model.ExamQuestionRow{
		Id: qid, ExamId: q.ExamId, Type: qt, Stem: req.Stem, Options: req.Options,
		Answer: req.Answer, Score: score, Required: required,
	})
}

// DeleteQuestion 删题。
func (l *ExamLogic) DeleteQuestion(qid int64, cu *class.CtxUser) error {
	q, err := l.svcCtx.ExamStore.GetQuestion(qid)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "题目不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(q.ExamId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	return l.svcCtx.ExamStore.DeleteQuestion(qid)
}

// SortQuestions 拖拽排序。
func (l *ExamLogic) SortQuestions(examId int64, req *exam.SortQuestionsReq, cu *class.CtxUser) error {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	if len(req.Order) == 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "排序数组不能为空"}
	}
	return l.svcCtx.ExamStore.SortQuestions(examId, req.Order)
}

// UpdateExam 更新考试基本信息。
func (l *ExamLogic) UpdateExam(examId int64, req *exam.UpdateExamReq, cu *class.CtxUser) error {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return be
	}
	if req.Title == "" {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "考试标题不能为空"}
	}
	start, err1 := time.ParseInLocation("2006-01-02T15:04", req.StartAt, time.Local)
	end, err2 := time.ParseInLocation("2006-01-02T15:04", req.EndAt, time.Local)
	if err1 != nil || err2 != nil {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "开始/结束时间格式应为 YYYY-MM-DDTHH:mm"}
	}
	if !end.After(start) {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "结束时间必须晚于开始时间"}
	}
	dur := req.DurationMin
	if dur <= 0 {
		dur = e.DurationMin
	}
	total := req.TotalScore
	if total <= 0 {
		total = e.TotalScore
	}
	return l.svcCtx.ExamStore.UpdateExam(&model.ExamRow{
		Id: examId, Title: req.Title, Description: req.Description,
		StartAt: start, EndAt: end, DurationMin: dur, TotalScore: total,
		AllowLate: req.AllowLate, TabLimit: req.TabLimit,
	})
}

// window 把 ExamRow 转成时间规则对象（规则集中在 examrules 包，只有一份实现）。
func windowOf(e *model.ExamRow) examrules.Window {
	return examrules.Window{
		StartAt:      e.StartAt,
		EndAt:        e.EndAt,
		DurationMin:  e.DurationMin,
		AllowLate:    e.AllowLate == 1,
		LateDeadline: e.LateDeadline.Time,
	}
}

// actorOf 用当前用户 + 班内角色构造授权主体。
func actorOf(cu *class.CtxUser, classRole string) authz.Actor {
	return authz.Actor{
		UserId:    cu.Id,
		Username:  cu.Username,
		Role:      cu.Role,
		ClassRole: classRole,
	}
}

// Submit 学生交卷（一次性，交完锁定）。
//
// 与改造前的区别：
//   - 时间判定交给 examrules：既看整场结束，也看"个人开考时间 + 时长"；
//   - 已经交过卷直接 409，不能再改答案（旧实现可以交完看答案再交一次刷分）；
//   - 交卷同时写一条成绩到成绩册，成绩页不再现算。
func (l *ExamLogic) Submit(examId int64, req *exam.SubmitReq, cu *class.CtxUser) (*exam.SubmitResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	if role == "teacher" || role == "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "老师无需参加考试"}
	}

	// 先看是否已经交过卷：409 比"静默覆盖"更符合直觉，也让前端能明确锁屏
	sub, err := l.svcCtx.ExamStore.GetMySubmission(examId, cu.Id)
	if err != nil {
		return nil, err
	}
	if sub != nil && sub.Status == model.SubmitStatusSubmitted {
		return nil, &class.BizError{Code: http.StatusConflict, Msg: examrules.ErrAlreadySubmitted.Error()}
	}

	now := time.Now()
	startedAt := time.Time{}
	if sub != nil && sub.StartedAt.Valid {
		startedAt = sub.StartedAt.Time
	}
	if err := windowOf(e).CheckSubmit(now, startedAt); err != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: err.Error()}
	}

	// 自动判客观题（抽成纯函数 ScoreExam，可单测）
	questions, _ := l.svcCtx.ExamStore.ListQuestions(examId)
	scoreRes := ScoreExam(questions, req.Answers)
	answersJSON, _ := json.Marshal(req.Answers)

	ok, err := l.svcCtx.ExamStore.SubmitOnce(&model.ExamSubmissionRow{
		ExamId: examId, UserId: cu.Id, Answers: string(answersJSON),
		AutoScore: nullInt(scoreRes.AutoScore),
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		// 并发场景：两个请求同时交卷，条件更新只有一个能成功
		return nil, &class.BizError{Code: http.StatusConflict, Msg: examrules.ErrAlreadySubmitted.Error()}
	}

	// 写入成绩册（幂等）。满分用考试设置的总分，缺失时用题目分值之和兜底。
	maxScore := float64(e.TotalScore)
	if maxScore <= 0 {
		maxScore = float64(sumQuestionScores(questions))
	}
	l.recordExamScore(e, cu.Id, float64(scoreRes.AutoScore), maxScore, 0)

	return &exam.SubmitResp{Message: "交卷成功", AutoScore: scoreRes.AutoScore}, nil
}

// sumQuestionScores 试卷满分之和（考试没配总分时的兜底）。
func sumQuestionScores(questions []*model.ExamQuestionRow) int {
	sum := 0
	for _, q := range questions {
		sum += q.Score
	}
	return sum
}

// recordExamScore 把考试得分写进成绩册。失败只记日志、不影响交卷本身：
// 交卷是学生的关键动作，不能因为统计表写失败就回滚整份答卷。
func (l *ExamLogic) recordExamScore(e *model.ExamRow, userId int64, score, maxScore float64, gradedBy int64) {
	itemId, err := l.svcCtx.ClassStore.EnsureGradeItem(e.ClassId, "exam", e.Id, e.Title, maxScore)
	if err != nil {
		logx.Errorf("写入成绩项失败 exam=%d: %v", e.Id, err)
		return
	}
	if err := l.svcCtx.ClassStore.SetGradeScore(itemId, userId, score, gradedBy); err != nil {
		logx.Errorf("写入成绩失败 exam=%d user=%d: %v", e.Id, userId, err)
	}
}

// Grade 老师手批（含授权校验）。
//
// 改造前这里没有任何鉴权，注释写着"老师能批任何考试提交"——
// 意味着任何登录用户都能改别人的分数。现在按班级老师身份校验。
func (l *ExamLogic) Grade(submissionId int64, req *exam.GradeReq, cu *class.CtxUser) error {
	if req.TeacherScore < 0 {
		return &class.BizError{Code: http.StatusBadRequest, Msg: "分数不能为负"}
	}
	sub, err := l.svcCtx.ExamStore.GetSubmission(submissionId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "提交记录不存在"}
	}
	e, err := l.svcCtx.ExamStore.GetExam(sub.ExamId)
	if err != nil {
		return &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	classRole, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return be
	}
	if err := actorOf(cu, classRole).Require(authz.ExamGrade, authz.Resource{ClassRole: classRole}); err != nil {
		return class.AsBizError(err)
	}

	if err := l.svcCtx.ExamStore.GradeSubmission(submissionId, req.TeacherScore, req.Comment, cu.Id); err != nil {
		return err
	}
	// 老师分覆盖自动分：最终成绩以老师批的为准
	maxScore := float64(e.TotalScore)
	if maxScore <= 0 {
		maxScore = 100
	}
	l.recordExamScore(e, sub.UserId, float64(req.TeacherScore), maxScore, cu.Id)
	return nil
}

// RecordViolation 前端上报一次切屏/离开页面（tab_limit 的落地方式）。
// 返回累计次数，前端据此提示；服务端只累加不判断，是否处理由老师决定。
func (l *ExamLogic) RecordViolation(examId int64, cu *class.CtxUser) (int, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return 0, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if _, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu); be != nil {
		return 0, be
	}
	return l.svcCtx.ExamStore.AddViolation(examId, cu.Id)
}

// Gradebook 单个学生在这场考试上的成绩（老师批阅页展示用）。
func nullInt(v int) (r sql.NullInt64) {
	r.Int64 = int64(v)
	r.Valid = true
	return
}

// StartExam 学生点"开始考试"：在 exam_submissions 写一行 started_at=now。
// 后续交卷 SubmitOnce 会基于唯一键 (exam_id,user_id) 一次性更新。
// 重复调用不会重置 started_at（model 层用 INSERT IGNORE），所以截止时间也不会被无限续期。
func (l *ExamLogic) StartExam(examId int64, cu *class.CtxUser) (*exam.StartExamResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	role, be := class.RequireClassMember(l.svcCtx, e.ClassId, cu)
	if be != nil {
		return nil, be
	}
	if role == "teacher" || role == "admin" {
		return nil, &class.BizError{Code: http.StatusForbidden, Msg: "老师无需参加考试"}
	}
	w := windowOf(e)
	if err := w.CheckStart(time.Now()); err != nil {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: err.Error()}
	}
	if err := l.svcCtx.ExamStore.StartSubmission(examId, cu.Id, time.Now()); err != nil {
		return nil, err
	}
	// 重新读一次：若学生之前已经开考过，INSERT IGNORE 不会覆盖，
	// 截止时间必须按"第一次开考时间"算，否则刷新页面就能续时长。
	sub, err := l.svcCtx.ExamStore.GetMySubmission(examId, cu.Id)
	if err != nil {
		return nil, err
	}
	started := startedAtOf(sub)
	if started.IsZero() {
		started = time.Now()
	}
	return &exam.StartExamResp{
		ExamId:      examId,
		StartedAt:   started.Format(time.RFC3339),
		DeadlineAt:  w.Deadline(started).Format(time.RFC3339),
		DurationMin: e.DurationMin,
	}, nil
}

// BatchAddQuestions 老师一次给考试加多道题（手动组卷）。
// 与 AddQuestion 单题接口互补：整份试卷录入时用这个接口，省 N 次 RTT。
func (l *ExamLogic) BatchAddQuestions(examId int64, req *exam.BatchAddQuestionsReq, cu *class.CtxUser) (*exam.BatchAddQuestionsResp, error) {
	e, err := l.svcCtx.ExamStore.GetExam(examId)
	if err != nil {
		return nil, &class.BizError{Code: http.StatusNotFound, Msg: "考试不存在"}
	}
	if be := class.RequireTeacher(l.svcCtx, e.ClassId, cu); be != nil {
		return nil, be
	}
	if len(req.Questions) == 0 {
		return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题目列表不能为空"}
	}
	rows := make([]*model.ExamQuestionRow, 0, len(req.Questions))
	for _, q := range req.Questions {
		if q.Stem == "" {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "题干不能为空"}
		}
		qt := q.Type
		if qt == "" {
			qt = "single"
		}
		if qt != "essay" && q.Answer == "" {
			return nil, &class.BizError{Code: http.StatusBadRequest, Msg: "选择题/判断题答案不能为空"}
		}
		score := q.Score
		if score <= 0 {
			score = 5
		}
		rows = append(rows, &model.ExamQuestionRow{
			ExamId: examId, Type: qt, Stem: q.Stem, Options: q.Options, Answer: q.Answer, Score: score,
		})
	}
	ids, err := l.svcCtx.ExamStore.BatchInsertQuestions(rows)
	if err != nil {
		return nil, err
	}
	return &exam.BatchAddQuestionsResp{Count: len(ids), Ids: ids}, nil
}
