package logic

// 超时自动收卷（服务端权威时间）。
//
// 背景：改造前倒计时完全由前端计算（基于 exam.end_at），
// 关掉页面/改本地时间就不受约束，服务端也不会有任何动作。
// 现在由后台任务每分钟扫一次"考试中"的记录，用 examrules 的同一套规则判定，
// 超过"开考时间 + 时长"就强制收卷（status=expired, auto_score=0）。

import (
	"time"

	"learningplatform/internal/app/exam/logic/examrules"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// autoSubmitInterval 扫描间隔。1 分钟足够：最坏情况是超时后 60 秒内被收卷。
const autoSubmitInterval = time.Minute

// StartAutoSubmitter 启动后台自动收卷（在 main 里调一次即可）。
func StartAutoSubmitter(svcCtx *svc.ServiceContext) {
	go func() {
		ticker := time.NewTicker(autoSubmitInterval)
		defer ticker.Stop()
		for range ticker.C {
			if n := RunAutoSubmitOnce(svcCtx); n > 0 {
				logx.Infof("自动收卷：本次处理 %d 份超时答卷", n)
			}
		}
	}()
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
	now := time.Now()
	n := 0
	for _, r := range rows {
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
		if !w.AutoSubmitDue(now, started) {
			continue
		}
		if err := svcCtx.ExamStore.MarkExpired(r.SubmissionId); err != nil {
			logx.Errorf("自动收卷失败 submission=%d: %v", r.SubmissionId, err)
			continue
		}
		n++
	}
	return n
}
