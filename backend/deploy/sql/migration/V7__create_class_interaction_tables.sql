-- Flyway V7：班级互动模块（公告/作业/讨论）

CREATE TABLE IF NOT EXISTS `announcements` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL,
  `title`      varchar(256)    NOT NULL,
  `content`    text            NOT NULL,
  `publisher_id` bigint unsigned NOT NULL COMMENT '发布者 user_id',
  `is_top`     tinyint(1)      NOT NULL DEFAULT 0 COMMENT '置顶',
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_class` (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级公告';

CREATE TABLE IF NOT EXISTS `assignments` (
  `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`    bigint unsigned NOT NULL,
  `course_id`   bigint unsigned NOT NULL DEFAULT 0 COMMENT '关联课程id，0=不关联',
  `title`       varchar(256)    NOT NULL,
  `description` text            NOT NULL,
  `deadline`    datetime        NULL COMMENT '截止时间',
  `total_score` int             NOT NULL DEFAULT 100,
  `publisher_id` bigint unsigned NOT NULL,
  `created_at` datetime         NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_class` (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='作业';

CREATE TABLE IF NOT EXISTS `assignment_submissions` (
  `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
  `assignment_id` bigint unsigned NOT NULL,
  `user_id`       bigint unsigned NOT NULL,
  `content`       text            COMMENT '文字提交内容',
  `file_url`      varchar(512)    DEFAULT '' COMMENT '提交文件URL',
  `score`         int             DEFAULT NULL COMMENT '得分',
  `comment`       varchar(512)    DEFAULT '' COMMENT '老师评语',
  `graded_by`     bigint unsigned DEFAULT NULL,
  `submitted_at`  datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `graded_at`     datetime        NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_assign_user` (`assignment_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='作业提交';

CREATE TABLE IF NOT EXISTS `discussions` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL,
  `user_id`    bigint unsigned NOT NULL,
  `title`      varchar(256)    NOT NULL,
  `content`    text            NOT NULL,
  `last_reply_at` datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '最后回复时间，用于排序',
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_class` (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='讨论帖';

CREATE TABLE IF NOT EXISTS `discussion_replies` (
  `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
  `discussion_id` bigint unsigned NOT NULL,
  `user_id`       bigint unsigned NOT NULL,
  `content`       text            NOT NULL,
  `created_at`    datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_discussion` (`discussion_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='讨论回复';
