-- Flyway V15：考试中心

CREATE TABLE IF NOT EXISTS `exams` (
  `id`             bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`       bigint unsigned NOT NULL,
  `title`          varchar(256)    NOT NULL,
  `description`    text,
  `start_at`       datetime        NOT NULL COMMENT '考试开始时间',
  `end_at`         datetime        NOT NULL COMMENT '考试结束时间',
  `duration_minutes` int           NOT NULL DEFAULT 60 COMMENT '考试时长（分钟），到时间自动交卷',
  `total_score`    int             NOT NULL DEFAULT 100,
  `created_by`     bigint unsigned NOT NULL,
  `created_at`     datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_class` (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级考试';

CREATE TABLE IF NOT EXISTS `exam_questions` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `exam_id`    bigint unsigned NOT NULL,
  `type`       varchar(16)      NOT NULL COMMENT 'single=单选, judge=判断, essay=主观',
  `stem`       varchar(512)    NOT NULL,
  `options`    varchar(1024)   NOT NULL DEFAULT '' COMMENT '选择题选项JSON数组',
  `answer`     varchar(64)     NOT NULL DEFAULT '' COMMENT '标准答案，主观题可为空',
  `score`      int             NOT NULL DEFAULT 5,
  PRIMARY KEY (`id`),
  KEY `idx_exam` (`exam_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='考试题目';

CREATE TABLE IF NOT EXISTS `exam_submissions` (
  `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
  `exam_id`       bigint unsigned NOT NULL,
  `user_id`       bigint unsigned NOT NULL,
  `answers`       text           COMMENT '学生作答JSON {questionId: answer}',
  `auto_score`    int            DEFAULT NULL COMMENT '客观题自动得分',
  `teacher_score` int            DEFAULT NULL COMMENT '老师手批总分（含主观题）',
  `comment`       varchar(512)   DEFAULT '',
  `started_at`    datetime,
  `submitted_at`  datetime       NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_exam_user` (`exam_id`, `user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='考试提交记录';
