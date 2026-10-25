package course

import (
	"context"
	"strings"
	"time"

	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// ===== 孤儿对象清理（网络波动兜底）=====
//
// 场景：客户端把封面/视频上传到 MinIO 成功后，HTTP 响应在网络波动中丢失，
// 课程存储里的 SetCover / AddVideos 没有执行（或执行了但客户端没收到响应），
// 于是 MinIO 里留下"课程不引用"的孤儿对象，界面上也看不到这些资源。
//
// 方案：后台定时扫描 MinIO 课程目录，删除「未被任何课程引用」且
// 「存在时间超过安全窗口」的对象。安全窗口避免误删刚上传、引用还没来得及写回的对象。

const (
	// orphanSweepInterval 扫描周期：每 10 分钟跑一次。
	orphanSweepInterval = 10 * time.Minute
	// orphanSafetyWindow 安全窗口：对象上传后至少保留这么久才可能被清理。
	// 覆盖"上传成功 → 引用写回"之间的正常间隔，防止误删。
	orphanSafetyWindow = 10 * time.Minute
)

// StartOrphanSweeper 启动后台孤儿清理协程（服务启动时调用一次）。
// MinIO 未初始化（Oss == nil）时直接返回，不影响服务启动。
func StartOrphanSweeper(svcCtx *svc.ServiceContext) {
	if svcCtx.Oss == nil {
		return
	}
	go func() {
		// 启动后先等一个周期再首跑，避免和"正在上传"的首次请求抢时间窗口
		ticker := time.NewTicker(orphanSweepInterval)
		defer ticker.Stop()
		for range ticker.C {
			if err := sweepOrphans(svcCtx); err != nil {
				logx.Errorf("孤儿对象清理失败: %v", err)
			}
		}
	}()
}

// sweepOrphans 扫描并清理孤儿对象。
// 独立函数便于单元测试直接调用（不依赖后台协程）。
func sweepOrphans(svcCtx *svc.ServiceContext) error {
	if svcCtx.Oss == nil {
		return nil
	}

	// 1. 收集当前所有被课程引用的对象名（封面原图 / 封面缩略图 / 视频）
	referenced := make(map[string]struct{})
	for _, c := range Store.List() {
		collectReferenced(referenced, c.CoverUrl, svcCtx.Config.Minio.Bucket)
		collectReferenced(referenced, c.CoverThumbUrl, svcCtx.Config.Minio.Bucket)
		for _, v := range c.Videos {
			collectReferenced(referenced, v.Url, svcCtx.Config.Minio.Bucket)
		}
	}

	// 2. 列出课程目录下的所有对象
	objects, err := svcCtx.Oss.ListObjects(context.Background(), "course/")
	if err != nil {
		return err
	}

	// 3. 删除「未被引用」且「超过安全窗口」的对象
	now := time.Now()
	removed := 0
	for _, obj := range objects {
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

// collectReferenced 把一条存储 URL 解析成桶内对象名并加入引用集合。
// URL 形如 http://endpoint/bucket/objectName，按 "/bucket/" 定位截取。
// 解析失败的 URL 直接忽略，不影响其他引用（也不会误删——引用缺失最多导致多删一条孤儿）。
func collectReferenced(set map[string]struct{}, url, bucket string) {
	if url == "" || bucket == "" {
		return
	}
	marker := "/" + bucket + "/"
	idx := strings.Index(url, marker)
	if idx < 0 {
		return
	}
	objectName := url[idx+len(marker):]
	if objectName != "" {
		set[objectName] = struct{}{}
	}
}
