// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package svc

import (
	"learningplatform/internal/config"
	"learningplatform/internal/model"
)

type ServiceContext struct {
	Config config.Config
	// UserStore 用户存储（内存版），登录/注册逻辑都从这里取数据
	UserStore *model.UserStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:    c,
		UserStore: model.NewUserStore(),
	}
}
