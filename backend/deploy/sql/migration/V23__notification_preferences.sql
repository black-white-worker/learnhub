-- 记录用户不同通知渠道的接收开关偏好，同一用户同一渠道仅一条配置
CREATE TABLE IF NOT EXISTS `notification_preferences` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id`    bigint unsigned NOT NULL,
  `channel`    varchar(32) NOT NULL DEFAULT 'inapp' COMMENT 'inapp/email',
  `enabled`    tinyint(1)  NOT NULL DEFAULT 1,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_channel` (`user_id`, `channel`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='通知偏好';
