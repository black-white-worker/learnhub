package course

// 本文件为 course 包测试提供公共 svcCtx 构造。
//
// 背景：upload_test_helper_test.go / orphansweeper_test.go 都在调用 newTestSvcCtx，
// 但该函数原本定义在 logic 包（createlogic_test.go），跨包取不到，
// 导致 course 包测试编译失败。这里补一份等价实现。

import (
	"learningplatform/internal/config"
	"learningplatform/internal/svc"
)

func newTestSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		Config: config.Config{
			Upload: config.UploadConf{
				MaxCoverBytes: 5 << 20,
				MaxVideoBytes: 200 << 20,
			},
			Minio: config.MinioConf{Bucket: "learningplatform"},
		},
		Oss: newFakeOss(),
	}
}
