-- Flyway 版本化迁移 V1：初始化 users 表
-- 本脚本由 Flyway 在 backend 启动前自动执行（docker-compose 里的 flyway 服务）。
-- 命名规则：V<版本号>__<描述>.sql，Flyway 按版本号顺序执行并记录在 flyway_schema_history 表。

-- 用户表：登录 / 注册功能使用
CREATE TABLE IF NOT EXISTS `users` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `username`   varchar(64)      NOT NULL COMMENT '用户名（唯一）',
  `password`   varchar(64)      NOT NULL COMMENT '密码（教学版明文存储）',
  `nickname`   varchar(64)      NOT NULL DEFAULT '' COMMENT '昵称',
  `created_at` datetime         NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '用户表';

-- 预置测试账号：admin / 123456（INSERT IGNORE 保证重复执行时不报错）
INSERT IGNORE INTO `users` (`username`, `password`, `nickname`)
VALUES ('admin', '123456', '管理员');
