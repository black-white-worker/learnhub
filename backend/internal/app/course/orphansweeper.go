package course

// ===== 孤儿对象清理（网络波动兜底）=====
//
// 场景：客户端把封面/视频/讲义上传到 MinIO 成功后，HTTP 响应在网络波动中丢失，
// 课程存储里的引用没有写回（或写了但客户端没收到响应），
// 于是 MinIO 里留下"课程不引用"的孤儿对象，界面上也看不到这些资源。
//
// 方案：后台定时扫描 MinIO 课程目录，删除「未被任何课程引用」且
// 「存在时间超过安全窗口」的对象。
//
// 安全约束（缺一不可，历史教训）：
//  1. 引用集合必须覆盖所有落库对象 —— 见 objectrefs.go 的 ReferencedObjects；
//  2. 只清理白名单前缀，未知前缀一律不碰（宁可留垃圾，不可误删数据）；
//  3. 安全窗口避免误删"刚上传、引用还没写回"的对象。

import (
	"context"
	"time"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// orphanSweepInterval 扫描周期：每 10 分钟跑一次。
	orphanSweepInterval = 10 * time.Minute
	// orphanSafetyWindow 安全窗口：对象上传后至少保留这么久才可能被清理。
	// 覆盖"上传成功 → 引用写回"之间的正常间隔，防止误删。
	orphanSafetyWindow = 10 * time.Minute
)

// StartOrphanSweeper 启动后台孤儿清理协程（服务启动时调用一次）。
//
// parent 用于优雅退出：收到 parent.Done() 后停止派发新任务并退出 goroutine，
// 避免在进程退出时被强杀、留下半截清理动作。
// 返回值是一个 done channel，调用方（main）在 server 优雅关闭后可 <-done
// 等待其真正退出。MinIO 未初始化（Oss == nil）时立即关闭 done 并返回，不影响启动。
func StartOrphanSweeper(parent context.Context, svcCtx *svc.ServiceContext) <-chan struct{} {
	done := make(chan struct{})
	if svcCtx.Oss == nil {
		close(done)
		return done
	}
	go func() {
		defer close(done)
		// 启动后先等一个周期再首跑，避免和"正在上传"的首次请求抢时间窗口
		ticker := time.NewTicker(orphanSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-parent.Done():
				logx.Infof("孤儿对象清理协程正在退出: %v", parent.Err())
				return
			case <-ticker.C:
				if err := sweepOrphans(svcCtx); err != nil {
					logx.Errorf("孤儿对象清理失败: %v", err)
				}
			}
		}
	}()
	return done
}

// collectReferencedObjects 汇总所有课程的引用对象。独立出来是为了能单测：
// "讲义必须在引用集合里"是曾经出过数据丢失事故的不变量。
func collectReferencedObjects(courses []*Course, bucket string) map[string]struct{} {
	referenced := make(map[string]struct{})
	for _, c := range courses {
		for _, name := range c.ReferencedObjects(bucket) {
			referenced[name] = struct{}{}
		}
	}
	return referenced
}

// sweepOrphans 扫描并清理孤儿对象。
// 独立函数便于单元测试直接调用（不依赖后台协程）。
func sweepOrphans(svcCtx *svc.ServiceContext) error {
	if svcCtx.Oss == nil {
		return nil
	}

	bucket := svcCtx.Config.Minio.Bucket
	referenced := collectReferencedObjects(Store.List(), bucket)

	objects, err := svcCtx.Oss.ListObjects(context.Background(), "course/")
	if err != nil {
		return err
	}

	now := time.Now()
	removed := 0
	for _, obj := range objects {
		// 白名单外的对象（导出文件、将来的新前缀）一律不碰：
		// 清理器只对自己完全理解的目录负责。
		if !IsManagedObject(obj.Name) {
			continue
		}
		if _, ok := referenced[obj.Name]; ok {
			continue // 正在被课程引用，不动
		}
		if now.Sub(obj.LastModified) < orphanSafetyWindow {
			continue // 太新：可能是刚上传、引用还没写回，给安全窗口
		}
		if err := svcCtx.Oss.RemoveObject(context.Background(), obj.Name); err != nil {
			logx.Errorf("清理孤儿对象 %s 失败: %v", obj.Name, err)
			continue
		}
		removed++
		logx.Infof("清理孤儿对象: %s", obj.Name)
	}
	if removed > 0 {
		logx.Infof("孤儿对象清理完成，共删除 %d 个", removed)
	}
	return nil
}
