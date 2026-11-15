package course

// ===== 视频后处理（faststart）worker 池 =====
//
// 改造前：上传接口直接 `go processFaststartAsync(v)` —— 协程不入账、不限并发、
// 进程一重启就永久丢失，而且几个大视频同时转码能把 CPU 打满。
//
// 现在：course_videos.process_status 就是队列本身。
//   - 上传只写一行 pending（接口秒回）；
//   - worker 用条件更新认领（UPDATE ... WHERE status='pending'，抢到才算我的），
//     多实例 / 多 worker 同时抢也只有一个成功，所以不需要引入 Redis 队列或 MQ；
//   - 进程重启后先跑一轮，把 pending 接上；崩溃遗留的 processing
//     靠 processed_at 超时判定退回 pending；
//   - 失败自动重试到上限后置为 failed，前端给"重试"按钮手工重新排队。
//
// 并发度只有 2：ffmpeg 转码是 CPU 密集，和在线请求抢核的代价远大于排队几秒。

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"learningplatform/internal/model"
	"learningplatform/internal/platform/concurrency"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// videoProcessInterval 兜底轮询周期。上传后会立刻唤醒一次，这里只兜住漏网。
	videoProcessInterval = time.Minute
	// videoProcessBatch 单轮最多处理多少个视频，避免积压时一轮跑几十分钟。
	videoProcessBatch = 8
	// videoProcessConcurrency 同时转码的视频数（见上面注释：CPU 密集，刻意压低）。
	videoProcessConcurrency = 2
	// videoProcessMaxAttempts 最多尝试次数（认领即计数）。3 次够覆盖
	// "ffmpeg 偶发失败 / MinIO 抖一下"，又不会让坏文件无限重跑。
	videoProcessMaxAttempts = 3
	// videoProcessStaleAfter processing 卡超过这么久就判定为"认领者已死"。
	// 必须明显大于 ffmpegTimeout(10 分钟) + 整片下载时间，否则会抢别的实例正在跑的活。
	videoProcessStaleAfter = 35 * time.Minute
	// videoProcessRoundTimeout 单轮总超时：8 个 / 2 并发 = 4 批，每批最多 10 分钟。
	// 超时后取消在跑的转码并把它们退回 pending（不是 failed），下一轮继续。
	videoProcessRoundTimeout = 45 * time.Minute
)

// videoProcessWake 上传后唤醒 worker：容量 1 的信号通道，
// 塞满了就丢弃（反正 worker 醒来一定会把队列扫空一轮）。
var videoProcessWake = make(chan struct{}, 1)

// WakeVideoProcessor 通知后处理 worker "有新活了"。
// 非阻塞：调用方是上传接口，绝不能因为 worker 忙而卡住响应。
func WakeVideoProcessor() {
	select {
	case videoProcessWake <- struct{}{}:
	default:
	}
}

// videoQueue 后处理 worker 需要的最小数据访问面（*model.CourseModel 实现它）。
// 抽出来是为了在测试里换掉整张表：状态机（认领 / 重试 / 升级 failed / 中断回队）
// 不该依赖真 MySQL 和真 ffmpeg 才能验证。
type videoQueue interface {
	ListPendingVideos(limit int) ([]*model.VideoRow, error)
	ClaimPendingVideo(id int64) (bool, error)
	FinishVideoProcess(id int64, status, newUrl, procErr string) error
	ResetStaleProcessingVideos(after time.Duration) (int64, error)
}

// faststartRun 转码实现，包级变量便于测试替换。
var faststartRun = ProcessVideoFaststart

// StartVideoProcessor 启动视频后处理 worker（在 main 里调一次）。
//
// parent 用于优雅退出：收到 parent.Done() 后取消在跑的转码并退出，
// 被取消的任务会退回 pending 由下个进程接上。
// 返回 done channel，main 在 server 关闭后 <-done 等它真正退出。
//
// 队列在 MySQL 里，所以没接 DB 或没接对象存储时直接不启动（立即关闭 done）。
// 这也意味着不接 MySQL 的开发模式（内存存储）不会有 faststart 版本，
// 视频仍按原 URL 播放，只是不能拖动进度条。
func StartVideoProcessor(parent context.Context, svcCtx *svc.ServiceContext) <-chan struct{} {
	done := make(chan struct{})
	if svcCtx.CourseStore == nil || svcCtx.Oss == nil {
		close(done)
		return done
	}

	// parent 一取消就中断在跑的转码：worker 主循环是单协程串行，
	// 若没有这个看门狗，一轮最长 45 分钟里关停信号只能干等。
	runCtx, cancelRun := context.WithCancel(parent)
	go func() {
		<-parent.Done()
		cancelRun()
	}()

	go func() {
		defer close(done)
		recoverStaleVideos(runCtx, svcCtx.CourseStore)
		// 启动先跑一轮：上一个进程留下的 pending（或没跑完的）立刻接上
		if n := RunVideoProcessOnce(runCtx, svcCtx); n > 0 {
			logx.Infof("视频后处理：启动轮次处理了 %d 个", n)
		}

		ticker := time.NewTicker(videoProcessInterval)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				logx.Infof("视频后处理 worker 正在退出: %v", parent.Err())
				return
			case <-videoProcessWake:
				RunVideoProcessOnce(runCtx, svcCtx)
			case <-ticker.C:
				RunVideoProcessOnce(runCtx, svcCtx)
			}
		}
	}()
	return done
}

// recoverStaleVideos 把崩溃/重启遗留的 processing 退回 pending。
// 单独一步是为了让日志里看得见"上次有几个任务中断了"。
func recoverStaleVideos(ctx context.Context, q videoQueue) {
	if ctx.Err() != nil {
		return
	}
	n, err := q.ResetStaleProcessingVideos(videoProcessStaleAfter)
	if err != nil {
		logx.Errorf("重置卡死的视频处理任务失败: %v", err)
		return
	}
	if n > 0 {
		logx.Infof("视频后处理：%d 个中断的任务已重新排队", n)
	}
}

// RunVideoProcessOnce 跑一轮：捞 pending → 逐个认领并转码，返回处理成功的数量。
// 导出来是为了在测试里手动触发一轮，不用等 ticker。
func RunVideoProcessOnce(ctx context.Context, svcCtx *svc.ServiceContext) int {
	if svcCtx.CourseStore == nil {
		return 0
	}
	return runVideoProcessRound(ctx, svcCtx, svcCtx.CourseStore)
}

// runVideoProcessRound 一轮的实际逻辑。
// 任务失败不影响同轮其他视频：单个视频的状态已经写回队列，
// 冒泡到 errgroup 只会连带取消同轮其他视频。
func runVideoProcessRound(ctx context.Context, svcCtx *svc.ServiceContext, q videoQueue) int {
	if ctx.Err() != nil {
		return 0
	}
	rows, err := q.ListPendingVideos(videoProcessBatch)
	if err != nil {
		logx.Errorf("拉取待处理视频失败: %v", err)
		return 0
	}
	if len(rows) == 0 {
		return 0
	}

	roundCtx, cancel := context.WithTimeout(ctx, videoProcessRoundTimeout)
	defer cancel()

	var succeeded int32
	_ = concurrency.ForEach(roundCtx, len(rows), videoProcessConcurrency, func(ctx context.Context, i int) error {
		row := rows[i]
		claimed, claimErr := q.ClaimPendingVideo(row.Id)
		if claimErr != nil {
			logx.Errorf("认领视频 %d 失败: %v", row.Id, claimErr)
			return nil
		}
		// 没抢到说明别的 worker / 实例已经在处理，直接跳过
		if !claimed {
			return nil
		}
		if processVideoForRow(ctx, svcCtx, q, row) {
			atomic.AddInt32(&succeeded, 1)
		}
		return nil
	})
	return int(atomic.LoadInt32(&succeeded))
}

// processVideoForRow 处理一个已认领的视频，并把结果写回状态表。
// 返回是否转码成功。
func processVideoForRow(ctx context.Context, svcCtx *svc.ServiceContext, q videoQueue, row *model.VideoRow) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("视频 %d 后处理 panic: %v", row.Id, r)
			finishFailed(q, row, errors.New("处理异常崩溃"))
			ok = false
		}
	}()

	// 老数据兜底：没有源对象名的行永远转不出 faststart 版本，
	// 直接标 failed 说明原因，不占着队列反复重试。
	if row.ObjectName == "" {
		logx.Errorf("视频 %d 缺少源对象名，无法转码", row.Id)
		if err := q.FinishVideoProcess(row.Id, model.VideoProcessFailed, "", "源对象名缺失，无法转码，请重新上传"); err != nil {
			logx.Errorf("视频 %d 写回失败状态失败: %v", row.Id, err)
		}
		return false
	}

	newURL, err := faststartRun(ctx, svcCtx, videoFromRow(row))
	if err == nil {
		if err := q.FinishVideoProcess(row.Id, model.VideoProcessDone, newURL, ""); err != nil {
			// 转码产物已经上传成功，只是状态没写回：下轮重跑会再产一份，
			// 多出来的对象由孤儿清理器回收，不影响播放。
			logx.Errorf("视频 %d 标记处理完成失败: %v", row.Id, err)
			return false
		}
		logx.Infof("视频 %d faststart 处理完成", row.Id)
		return true
	}

	// ctx 被取消（关进程 / 整轮超时）不是视频的错：退回 pending 等下一轮，
	// 不占用重试次数上限。
	if ctx.Err() != nil {
		if ferr := q.FinishVideoProcess(row.Id, model.VideoProcessPending, "", "处理被中断（服务退出或本轮超时），已重新排队"); ferr != nil {
			logx.Errorf("视频 %d 退回队列失败: %v", row.Id, ferr)
		}
		logx.Infof("视频 %d 处理被中断，已重新排队", row.Id)
		return false
	}
	finishFailed(q, row, err)
	return false
}

// finishFailed 记录一次失败：还有重试机会就退回 pending，否则置 failed。
// 认领时 attempts 已经 +1，所以这里比较的是"本次是第几次尝试"。
func finishFailed(q videoQueue, row *model.VideoRow, procErr error) {
	attempts := row.ProcessAttempts + 1
	status := model.VideoProcessPending
	if attempts >= videoProcessMaxAttempts {
		status = model.VideoProcessFailed
	}
	if err := q.FinishVideoProcess(row.Id, status, "", procErr.Error()); err != nil {
		logx.Errorf("视频 %d 写回失败状态失败: %v", row.Id, err)
	}
	logx.Infof("视频 %d faststart 失败（第 %d 次尝试，状态 %s）：%v", row.Id, attempts, status, procErr)
}
