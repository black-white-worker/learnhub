// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package svc

import (
	"learningplatform/internal/config"
	"learningplatform/internal/model"
	"learningplatform/internal/oss"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config    config.Config
	Oss       oss.ObjectStorage
	UserStore *model.UserModel
	// 课程模块 MySQL 数据访问层（课程元数据持久化）
	CourseStore *model.CourseModel
	// Redis 缓存客户端，登录成功后把 token 写入 Redis
	Redis *redis.Redis
}

func NewServiceContext(c config.Config) *ServiceContext {
	sc := &ServiceContext{
		Config:    c,
		UserStore: model.NewUserModel(sqlx.NewMysql(c.Mysql.DataSource)),
		CourseStore: model.NewCourseModel(sqlx.NewMysql(c.Mysql.DataSource)),
		Redis:     redis.MustNewRedis(c.Redis),
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
