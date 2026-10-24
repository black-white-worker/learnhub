-- Flyway V10：班级签到

CREATE TABLE IF NOT EXISTS `class_signin` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL,
  `teacher_id` bigint unsigned NOT NULL COMMENT '发起老师 user_id',
  `code`       varchar(16)     NOT NULL COMMENT '签到码，6位数字',
  `expires_at` datetime        NOT NULL COMMENT '签到截止时间',
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_class` (`class_id`),
  KEY `idx_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级签到（一次发起一条）';

CREATE TABLE IF NOT EXISTS `signin_records` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `signin_id`  bigint unsigned NOT NULL,
  `user_id`    bigint unsigned NOT NULL,
  `signed_at`  datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_signin_user` (`signin_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='签到记录';
