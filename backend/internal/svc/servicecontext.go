// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package svc

import (
	"learningplatform/internal/config"
	"learningplatform/internal/model"
	"learningplatform/internal/oss"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config config.Config
	// UserStore 用户存储（内存版），登录/注册逻辑都从这里取数据
	UserStore *model.UserStore
	// Oss 对象存储（MinIO）。
	// 配置了 Minio.Endpoint 才会初始化；未配置或初始化失败时为 nil，
	// 上传接口会返回明确错误，而不是 panic。
	Oss oss.ObjectStorage
}

func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{
		Config:    c,
		UserStore: model.NewUserStore(),
	}

	// MinIO 配置了 Endpoint 才初始化对象存储。
	// 初始化失败只记日志不 panic（比如启动时 MinIO 还没起来），接口层再报错。
	if c.Minio.Endpoint != "" {
		store, err := oss.NewMinioStore(c.Minio)
		if err != nil {
			logx.Errorf("初始化 MinIO 对象存储失败: %v", err)
		} else {
			sc.Oss = store
		}
	}

	return sc
}
