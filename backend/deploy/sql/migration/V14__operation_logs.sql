-- Flyway V14：操作日志表（审计关键写操作）

CREATE TABLE IF NOT EXISTS `operation_logs` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id`    bigint unsigned DEFAULT NULL,
  `username`   varchar(64)     NOT NULL DEFAULT '',
  `action`     varchar(64)     NOT NULL COMMENT '如 create_class / delete_class / publish_notice / login',
  `target`     varchar(256)    NOT NULL DEFAULT '' COMMENT '操作对象，如 class:2',
  `detail`      varchar(512)    NOT NULL DEFAULT '',
  `ip`         varchar(64)     NOT NULL DEFAULT '',
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_time` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='操作日志';
