-- Flyway V11：作业客观题（选择题/判断题自动判分）

CREATE TABLE IF NOT EXISTS `assignment_questions` (
  `id`           bigint unsigned NOT NULL AUTO_INCREMENT,
  `assignment_id` bigint unsigned NOT NULL,
  `type`         varchar(16)     NOT NULL COMMENT 'single=单选, judge=判断',
  `stem`         varchar(512)    NOT NULL COMMENT '题干',
  `options`      varchar(1024)   NOT NULL DEFAULT '' COMMENT '选项JSON，如 ["A..","B.."]，判断题为空',
  `answer`       varchar(64)     NOT NULL COMMENT '正确答案：A/B/C/D 或 true/false',
  `score`        int             NOT NULL DEFAULT 5 COMMENT '该题分值',
  `created_at`   datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_assignment` (`assignment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='作业客观题';

-- 提交记录加 answers 字段（学生作答JSON）和 auto_score（客观题自动得分）
ALTER TABLE `assignment_submissions`
  ADD COLUMN `answers` varchar(2048) NOT NULL DEFAULT '' COMMENT '客观题作答JSON' AFTER `content`,
  ADD COLUMN `auto_score` int DEFAULT NULL COMMENT '客观题自动得分' AFTER `score`;
