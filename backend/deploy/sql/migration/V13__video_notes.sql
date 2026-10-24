-- Flyway V13：视频笔记（学生在视频时间点做笔记）

CREATE TABLE IF NOT EXISTS `video_notes` (
  `id`        bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id`   bigint unsigned NOT NULL,
  `course_id` bigint unsigned NOT NULL,
  `video_id`  bigint unsigned NOT NULL,
  `timestamp_seconds` int NOT NULL DEFAULT 0 COMMENT '笔记对应的视频时间点（秒）',
  `content`   varchar(1024) NOT NULL,
  `created_at` datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_video` (`user_id`, `course_id`, `video_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='视频笔记';
