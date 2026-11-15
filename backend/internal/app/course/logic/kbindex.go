package logic

// 讲义 → 知识库索引的联动。
//
// 为什么放在 logic 而不是 handler：上传/删除都可能在别处被调用（批量导入、
// 后台任务），索引联动必须跟着业务动作走，而不是跟着某条 HTTP 路由走。
//
// 两条硬约束：
//  1. **不能阻塞用户请求**：解析一份 1MB 的 docx 要几十秒到几分钟，
//     上传接口等不起。这里全部丢到 goroutine，失败只记日志；
//  2. **用 context.Background 而不是请求 ctx**：请求一结束 r.Context() 就被取消，
//     派生出来的调用会立刻失败 —— 表面上传成功，实际从没入库。

import (
	"context"
	"time"

	"learningplatform/internal/app/user"
	"learningplatform/internal/platform/aiservice"
	"learningplatform/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// kbIndexTimeout 后台索引动作的总超时。灌库本身在 ai-service 里异步跑，
// 这里等的是「提交回执 / 清理完成」，几十毫秒到几秒；30 秒是防御性上限。
const kbIndexTimeout = 30 * time.Second

// actorFromCtx 取发起人，用于让 ai-service 侧记录「谁传的这份讲义」。
// 拿不到（内部调用/测试）时留空，不影响灌库。
func actorFromCtx(ctx context.Context) aiservice.Actor {
	if u := user.FromContext(ctx); u != nil {
		return aiservice.Actor{UserId: u.Id, Role: u.Role}
	}
	return aiservice.Actor{}
}

// triggerKnowledgeIngest 讲义上传成功后异步灌库。
// AI 未启用（没配 Url / Token）时静默跳过：答疑本来就用不了，不该报错。
func triggerKnowledgeIngest(svcCtx *svc.ServiceContext, ctx context.Context, courseId, resourceId int64) {
	if svcCtx == nil || svcCtx.AI == nil || !svcCtx.AI.Enabled() {
		return
	}
	actor := actorFromCtx(ctx)
	go func() {
		callCtx, cancel := context.WithTimeout(context.Background(), kbIndexTimeout)
		defer cancel()
		receipt, err := svcCtx.AI.IngestCourse(callCtx, actor, courseId, resourceId)
		if err != nil {
			// 只记资源 id 和错误类型：讲义名可能含学生信息，不进日志
			logx.Errorf("课程 %d 资源 %d 提交知识库灌入失败：%v", courseId, resourceId, err)
			return
		}
		if receipt.Accepted && !receipt.Queued {
			logx.Infof("课程 %d 资源 %d 已提交知识库灌入", courseId, resourceId)
			return
		}
		// 同一门课串行灌库：前一份还在解析时本次排队，解析完会自动接着跑
		logx.Infof("课程 %d 资源 %d 已排队等待灌库（%s）",
			courseId, resourceId, receipt.Reason)
	}()
}

// triggerKnowledgePurge 讲义删除后异步清理索引，避免「删掉的资料还能被引用」。
func triggerKnowledgePurge(svcCtx *svc.ServiceContext, ctx context.Context, courseId, resourceId int64) {
	if svcCtx == nil || svcCtx.AI == nil || !svcCtx.AI.Enabled() {
		return
	}
	actor := actorFromCtx(ctx)
	go func() {
		callCtx, cancel := context.WithTimeout(context.Background(), kbIndexTimeout)
		defer cancel()
		if err := svcCtx.AI.PurgeResource(callCtx, actor, courseId, resourceId); err != nil {
			logx.Errorf("课程 %d 资源 %d 清理知识库索引失败：%v", courseId, resourceId, err)
		}
	}()
}
