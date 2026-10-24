package class

import (
	"context"
	"learningplatform/internal/svc"
	"net/http"
	"time"
)

// logOperation 记录一条操作日志（异步写库，失败静默）。
// 用于审计关键写操作：建班、删班、发公告、发作业、登录等。
func logOperation(svcCtx *svc.ServiceContext, r *http.Request, userId int64, username, action, target, detail string) {
	ip := r.RemoteAddr
	go func() {
		// 异步写，不阻塞主流程；失败静默忽略
		_, _ = svcCtx.DB.ExecCtx(context.Background(),
			"INSERT INTO operation_logs (user_id, username, action, target, detail, ip, created_at) VALUES (?,?,?,?,?,?,?)",
			userId, username, action, target, detail, ip, time.Now())
	}()
}
