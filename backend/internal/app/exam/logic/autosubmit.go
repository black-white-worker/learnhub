package logic

// 超时自动收卷（服务端权威时间）。
//
// 背景：改造前倒计时完全由前端计算（基于 exam.end_at），
// 关掉页面/改本地时间就不受约束，服务端也不会有任何动作。
// 现在由后台任务每分钟扫一次"考试中"的记录，用 examrules 的同一套规则判定，
// 超过"开考时间 + 时长"就强制收卷。
//
// 收卷时按"最后一次草稿"判分：前端每 15 秒 tick 一次草稿，
// 所以断网/崩浏览器不会再直接变成 0 分（旧实现只改状态，不判分）。
//
// 本轮性能改造（B 端后台任务的三条纪律）：
//
//  1. 不按份回查：草稿随扫描 SQL 一起带出，少 N 次 SELECT；
//  2. 按考试批量取数：同一场考试的题目只查 1 次、成绩项只 upsert 1 次，
//     而不是每个学生各查一遍（40 人超时 = 40 次重复的 ListQuestions）；
//  3. 有界并发：写库按 4 路并发，既缩短一轮耗时，又不会和在线请求抢满连接池；
//     整轮有 5 分钟超时，超时后不再派发新任务（已在执行的查询会自然结束），
//     避免一次异常把后台 goroutine 永久挂住。
//
// 并发安全：ExpireSubmission 是条件更新（WHERE status='started'），
// 学生恰好在同一秒交卷时只有一个能成功；失败的那条不会再写成绩册。

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"learningplatform/internal/domain/examrules"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/concurrency"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// autoSubmitInterval 扫描间隔。1 分钟足够：最坏情况是超时后 60 秒内被收卷。
const autoSubmitInterval = time.Minute

const (
	// autoSubmitConcurrency 收卷写库的并发度上限（不要超过连接池的零头）。
	autoSubmitConcurrency = 4
	// autoSubmitTimeout 单轮扫描的总超时：超时后停止派发新任务，宁可下一轮再补。
	autoSubmitTimeout = 5 * time.Minute
)

// StartAutoSubmitter 启动后台自动收卷（在 main 里调一次即可）。
//
// parent 用于优雅退出：收到 parent.Done() 后停止扫描并退出 goroutine，
// 避免在进程退出时被强杀、留下半截收卷动作。
// 返回值是一个 done channel，调用方（main）在 server 优雅关闭后可 <-done
// 等待其真正退出。
func StartAutoSubmitter(parent context.Context, svcCtx *svc.ServiceContext) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(autoSubmitInterval)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				logx.Infof("自动收卷协程正在退出: %v", parent.Err())
				return
			case <-ticker.C:
				if n := RunAutoSubmitOnce(svcCtx); n > 0 {
					logx.Infof("自动收卷：本次处理 %d 份超时答卷", n)
				}
			}
		}
	}()
	return done
}

// dueExamContext 一轮收卷里"按考试维度共享"的数据：题目、满分、成绩项。
// 同一场考试的所有超时学生复用这一份，不再每人查一遍。
type dueExamContext struct {
	classId   int64
	title     string
	maxScore  float64
	itemId    int64
	questions []*model.ExamQuestionRow
}

// RunAutoSubmitOnce 执行一轮扫描，返回被强制收卷的数量。
// 单独导出是为了能在测试里手动触发一轮，不用等 ticker。
func RunAutoSubmitOnce(svcCtx *svc.ServiceContext) int {
	if svcCtx.ExamStore == nil {
		return 0
	}
	rows, err := svcCtx.ExamStore.ListStartedSubmissions()
	if err != nil {
		logx.Errorf("扫描超时答卷失败: %v", err)
		return 0
	}

	// 先在内存里用同一套规则筛出"确实该收"的，再进入写库阶段。
	now := time.Now()
	due := make([]*model.OverdueSubmission, 0, len(rows))
	for _, r := range rows {
		if isOverdue(r, now) {
			due = append(due, r)
		}
	}
	if len(due) == 0 {
		return 0
	}

	exams := prepareDueExams(svcCtx, due)
	if exams == nil {
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), autoSubmitTimeout)
	defer cancel()

	var done atomic.Int64
	ferr := concurrency.ForEach(ctx, len(due), autoSubmitConcurrency, func(ctx context.Context, i int) error {
		r := due[i]
		e := exams[r.ExamId]
		if e == nil {
			return nil
		}
		expired, err := expireOne(svcCtx, r, e)
		if err != nil {
			// 单份失败只记日志，不影响同批其它答卷
			logx.Errorf("自动收卷失败 submission=%d: %v", r.SubmissionId, err)
			return nil
		}
		if expired {
			done.Add(1)
		}
		return nil
	})
	if ferr != nil && ctx.Err() != nil {
		logx.Errorf("自动收卷本轮超时中止（已完成 %d 份）: %v", done.Load(), ferr)
	}
	return int(done.Load())
}

// isOverdue 用 examrules 判定这一份是否已经超时（与交卷/开考共用同一套规则）。
func isOverdue(r *model.OverdueSubmission, now time.Time) bool {
	w := examrules.Window{
		StartAt:      r.StartAt,
		EndAt:        r.EndAt,
		DurationMin:  r.DurationMin,
		AllowLate:    r.AllowLate == 1,
		LateDeadline: r.LateDeadline.Time,
	}
	var started time.Time
	if r.StartedAt.Valid {
		started = r.StartedAt.Time
	}
	return w.AutoSubmitDue(now, started)
}

// prepareDueExams 批量准备"按考试共享"的数据：题目一次取回，成绩项每场只建一次。
// 任何一步失败都放弃本轮（没有题目就无法判分，继续收卷只会写 0 分）。
func prepareDueExams(svcCtx *svc.ServiceContext, due []*model.OverdueSubmission) map[int64]*dueExamContext {
	ids := make([]int64, 0, len(due))
	seen := make(map[int64]struct{}, len(due))
	for _, r := range due {
		if _, ok := seen[r.ExamId]; ok {
			continue
		}
		seen[r.ExamId] = struct{}{}
		ids = append(ids, r.ExamId)
	}

	questions, err := svcCtx.ExamStore.ListQuestionsByExamIds(ids)
	if err != nil {
		logx.Errorf("批量读取考试题目失败: %v", err)
		return nil
	}

	exams := make(map[int64]*dueExamContext, len(ids))
	for _, r := range due {
		if _, ok := exams[r.ExamId]; ok {
			continue
		}
		e := &dueExamContext{
			classId:   r.ClassId,
			title:     r.Title,
			questions: questions[r.ExamId],
		}
		if r.TotalScore > 0 {
			e.maxScore = float64(r.TotalScore)
		} else {
			e.maxScore = float64(sumQuestionScores(e.questions))
		}
		// 成绩项每场考试只 ensure 一次（内部是 upsert + 查 id 两条 SQL）。
		if e.maxScore > 0 && svcCtx.ClassStore != nil {
			itemId, eerr := svcCtx.ClassStore.EnsureGradeItem(e.classId, "exam", r.ExamId, e.title, e.maxScore)
			if eerr != nil {
				logx.Errorf("写入成绩项失败 exam=%d: %v", r.ExamId, eerr)
			} else {
				e.itemId = itemId
			}
		}
		exams[r.ExamId] = e
	}
	return exams
}

// expireOne 收一份超时答卷：按草稿判分 → 标记 expired → 写成绩册。
//
// 顺序说明：先算分再落库，避免"状态已锁但分数还是空的"中间态；
// 即使成绩册写入失败，答卷本身也已经正确落库（记录日志即可）。
// 返回值表示"本次调用确实完成了收卷"（并发交卷时可能是 false）。
func expireOne(svcCtx *svc.ServiceContext, r *model.OverdueSubmission, e *dueExamContext) (bool, error) {
	answers := map[string]string{}
	if r.Answers != "" {
		// 草稿解析失败按空卷处理：宁可 0 分，也不能因为脏数据卡住收卷
		_ = json.Unmarshal([]byte(r.Answers), &answers)
	}
	score := ScoreExam(e.questions, answers).AutoScore

	expired, err := svcCtx.ExamStore.ExpireSubmission(r.SubmissionId, score)
	if err != nil {
		return false, err
	}
	if !expired {
		// 条件更新 0 行：学生刚好在这期间交卷了，成绩以他的答卷为准，不能覆盖。
		return false, nil
	}

	if e.itemId > 0 && svcCtx.ClassStore != nil {
		if gerr := svcCtx.ClassStore.SetGradeScore(e.itemId, r.UserId, float64(score), 0); gerr != nil {
			// 成绩册写失败不影响收卷结果，记录后本轮继续
			logx.Errorf("自动收卷写成绩失败 submission=%d: %v", r.SubmissionId, gerr)
		}
	}
	return true, nil
}
