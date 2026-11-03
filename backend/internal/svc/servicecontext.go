// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package svc

import (
	"learningplatform/internal/config"
	"learningplatform/internal/model"
	"learningplatform/internal/platform/oss"
	"learningplatform/internal/platform/quota"
	"learningplatform/internal/platform/ws"

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
	// 班级模块数据访问层
	ClassStore *model.ClassModel
	// 考试中心数据访问层
	ExamStore *model.ExamModel
	// 分片上传会话数据访问层
	UploadStore *model.UploadSessionModel
	// Redis 缓存客户端，登录成功后把 token 写入 Redis
	Redis *redis.Redis
	// 上传配额限制器（按用户 + 自然日计数，DayQuota=0 时不限制）
	Quota *quota.Limiter
	// DB 暴露底层 sqlx 连接，供轻量表（操作日志等）直接读写
	DB sqlx.SqlConn
	// 通知偏好数据访问层（对应 Flyway V23 的 notification_preferences 表）
	NotificationPrefStore *model.NotificationPrefModel
	// 实时通知 WebSocket 连接中心（单例，Run 协程常驻整个进程生命周期）
	NotifHub *ws.Hub
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.Mysql.DataSource)

	// 实时通知 Hub：启动唯一的所有者 goroutine，常驻整个进程生命周期。
	// 所有连接的增删与消息广播都经 channel 串行处理（见 platform/ws）。
	hub := ws.NewHub()
	go hub.Run()

	sc := &ServiceContext{
		Config:      c,
		UserStore:   model.NewUserModel(conn),
		CourseStore: model.NewCourseModel(conn),
		ClassStore:  model.NewClassModel(conn),
		ExamStore:   model.NewExamModel(conn),
		UploadStore: model.NewUploadSessionModel(conn),
		Redis:       redis.MustNewRedis(c.Redis),
		DB:          conn,
		// 通知偏好与实时推送
		NotificationPrefStore: model.NewNotificationPrefModel(conn),
		NotifHub:              hub,
	}
	// 上传配额：跟着 Redis 走，上限来自配置（0 表示不限制）
	sc.Quota = quota.New(sc.Redis, c.Upload.DailyQuotaBytesPerUser)

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
