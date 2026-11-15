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
//  3. 安全窗口避免误删"刚上传、引用还没写回"的对象；
//  4. 引用集合不可信就整轮跳过 —— 见 sweepOrphans 的两道硬闸。
//     2026-11-07 线上事故就是第 4 条缺失造成的：courses 查询因列数不匹配报错，
//     Store.List() 把错误吞成空切片，清理器据此把一门正常课程的封面和视频全删了，
//     而桶没开版本控制，数据不可恢复。
//
// 删错的代价是永久的，留垃圾的代价只是占点磁盘 —— 所有判断都按这个不对称来定。

import (
	"context"
	"fmt"
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
	// orphanMassDeleteMinCount 熔断阈值之一：单轮删除数少于这个数不触发熔断。
	// 正常上传失败的残留是零星的，一次删几十个说明引用集合本身出了问题。
	orphanMassDeleteMinCount = 10
	// orphanMassDeleteRatio 熔断阈值之二：候选占托管对象的比例超过它即熔断。
	orphanMassDeleteRatio = 0.5
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
	courses, err := Store.List()
	if err != nil {
		// 硬闸一：引用集合建不出来就整轮跳过。
		// 这里绝不能"记个日志继续删"——拿不到课程 = 所有文件看起来都像孤儿。
		return fmt.Errorf("跳过本轮清理：读取课程列表失败: %w", err)
	}

	objects, err := svcCtx.Oss.ListObjects(context.Background(), "course/")
	if err != nil {
		return err
	}

	managed := 0
	for _, obj := range objects {
		if IsManagedObject(obj.Name) {
			managed++
		}
	}
	if len(courses) == 0 && managed > 0 {
		// 硬闸二：库里有文件却一门课都查不到。
		// 真实场景（课程全删了）很少，更常见的是连错库 / 空 schema / 查询异常，
		// 而这两种情况误删是不可逆的，所以直接跳过，等操作员确认后再清理。
		logx.Errorf("跳过本轮孤儿清理：课程表为空但 MinIO 仍有 %d 个托管对象。"+
			"若确实删除了全部课程，请人工清理 course/ 前缀", managed)
		return nil
	}
	referenced := collectReferencedObjects(courses, bucket)

	now := time.Now()
	candidates := make([]string, 0, len(objects))
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
		candidates = append(candidates, obj.Name)
	}

	// 硬闸三：熔断。一轮要删掉托管对象的一半以上（且不少于 10 个）说明
	// 引用集合和实际数据严重对不上——多半是引用解析或课程查询坏了，而不是真孤儿。
	// 先撤下来等人工看，也不要"删一半算一半"：要么按可信引用删，要么一个都不删。
	if len(candidates) >= orphanMassDeleteMinCount &&
		float64(len(candidates)) > float64(managed)*orphanMassDeleteRatio {
		logx.Errorf("跳过本轮孤儿清理：候选 %d 个 / 托管对象共 %d 个，超过 %.0f%% 熔断阈值，"+
			"疑似引用集合不可信（课程 %d 门，引用 %d 条），请核对后再放开",
			len(candidates), managed, orphanMassDeleteRatio*100, len(courses), len(referenced))
		return nil
	}

	removed := 0
	for _, name := range candidates {
		if err := svcCtx.Oss.RemoveObject(context.Background(), name); err != nil {
			logx.Errorf("清理孤儿对象 %s 失败: %v", name, err)
			continue
		}
		removed++
		logx.Infof("清理孤儿对象: %s", name)
	}
	if removed > 0 {
		logx.Infof("孤儿对象清理完成，共删除 %d 个（候选 %d 个，托管对象共 %d 个）", removed, len(candidates), managed)
	}
	return nil
}
