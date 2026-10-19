-- 学习平台初始化 SQL
-- 该脚本由 Docker MySQL 容器首次启动时自动执行（挂载到 /docker-entrypoint-initdb.d）

-- 强制本次会话使用 utf8mb4，避免容器默认 latin1 连接导致中文乱码（双重编码）
SET NAMES utf8mb4;

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

-- 预置测试账号：admin / 123456（INSERT IGNORE 保证重复初始化时不报错）
INSERT IGNORE INTO `users` (`username`, `password`, `nickname`)
VALUES ('admin', '123456', '管理员');
