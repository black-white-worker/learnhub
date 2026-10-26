package logic

import (
	"learningplatform/internal/svc"
)

// 历史测试 helper：coveruploadlogic_test.go 引用 newUploadTestSvcCtx，
// 但它原本定义在 course 包（跨包），导致 logic 包测试编译失败。
// 这里在 logic 包加一个同名 wrapper，复用 newTestSvcCtx。
func newUploadTestSvcCtx() *svc.ServiceContext {
	return newTestSvcCtx()
}
