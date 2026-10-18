// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.2

package config

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/rest"
)

type Config struct {
	rest.RestConf

	// Mysql 数据库连接配置（DSN 由环境变量 MYSQL_DSN 注入）
	Mysql struct {
		DataSource string
	}

	// Redis 缓存配置（登录 token 等）
	Redis redis.RedisConf
}
