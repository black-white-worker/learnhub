package class

import (
	"context"
	"learningplatform/internal/svc"
	"time"
)

// LogOperation 记录一条操作日志（异步写库，失败静默）。
func LogOperation(svcCtx *svc.ServiceContext, ip string, userId int64, username, action, target, detail string) {
	go func() {
		// 异步写，不阻塞主流程；失败静默忽略
		_, _ = svcCtx.DB.ExecCtx(context.Background(),
			"INSERT INTO operation_logs (user_id, username, action, target, detail, ip, created_at) VALUES (?,?,?,?,?,?,?)",
			userId, username, action, target, detail, ip, time.Now())
	}()
}
