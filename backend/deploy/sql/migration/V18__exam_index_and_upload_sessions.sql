-- Flyway V18：考试模块补强 + 分片上传会话表

-- 1. exam_submissions 加 started_at 索引
--    老师端要查"开考了但还没交卷"的学生列表，按 started_at 命中比全表扫快。
ALTER TABLE `exam_submissions`
  ADD INDEX `idx_started_at` (`started_at`);

-- 2. 分片上传会话表
--    后端只在 init / complete 时写一行，分片本身由前端直传 MinIO，不流经后端。
--    upload_id 是 MinIO 返回的唯一标识，object_name 是桶内最终路径。
CREATE TABLE IF NOT EXISTS `upload_sessions` (
  `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
  `upload_id`     varchar(128)     NOT NULL COMMENT 'MinIO Multipart Upload ID',
  `course_id`      bigint unsigned NOT NULL,
  `object_name`    varchar(512)     NOT NULL COMMENT '桶内最终路径，如 course/1/videos/xxx.mp4',
  `file_name`      varchar(256)     NOT NULL DEFAULT '' COMMENT '原始文件名（前端回显用）',
  `file_size`      bigint unsigned NOT NULL DEFAULT 0 COMMENT '总大小（字节），用于校验是否全部传完',
  `content_type`   varchar(64)      NOT NULL DEFAULT 'video/mp4',
  `status`         varchar(16)      NOT NULL DEFAULT 'init' COMMENT 'init/completed/aborted',
  `expire_at`      datetime         NOT NULL COMMENT '超时自动清理（init 后 24 小时）',
  `created_by`     bigint unsigned NOT NULL,
  `created_at`     datetime         NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `completed_at`   datetime         DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_upload_id` (`upload_id`),
  KEY `idx_course` (`course_id`),
  KEY `idx_expire` (`expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='分片上传会话（MinIO Multipart）';
