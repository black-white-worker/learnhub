-- Flyway V25：主观题 AI 初评结果
--
-- 为什么要单独一张表，而不是往 assignment_submissions 上加几列：
--   1. AI 分数是**建议**，不是成绩。成绩册里那一列（score / comment）只能由老师落笔，
--      混在同一行里迟早有人写 SQL 把 AI 分当成交了的成绩。
--   2. 一次批改会重跑（换模型、改提示词、失败重试），历史结果要能覆盖而不影响提交本身。
--   3. 加列会让"读提交"这条最热路径多带几个大字段（逐题评语可能很长）。
--
-- 主键用 submission_id：一份提交最多一条最新 AI 初评，重跑就 UPSERT。
CREATE TABLE IF NOT EXISTS `assignment_ai_grades` (
  `submission_id` bigint unsigned NOT NULL COMMENT '对应的作业提交 id',
  `assignment_id` bigint unsigned NOT NULL DEFAULT 0 COMMENT '作业 id（冗余，便于按作业统计）',
  `course_id`     bigint unsigned NOT NULL DEFAULT 0 COMMENT '课程 id（冗余）',
  `student_id`    bigint unsigned NOT NULL DEFAULT 0 COMMENT '学生 user id（冗余）',
  `task_id`       varchar(64)     NOT NULL DEFAULT '' COMMENT 'ai-service 批改任务 id',
  `status`        varchar(16)     NOT NULL DEFAULT 'processing' COMMENT 'processing/done/failed/manual',
  `score`         int             NOT NULL DEFAULT 0 COMMENT 'AI 给出的主观题合计得分',
  `max_score`     int             NOT NULL DEFAULT 0 COMMENT 'AI 批改题目合计满分',
  `comment`       varchar(1024)   NOT NULL DEFAULT '' COMMENT 'AI 总评（一句话，供老师参考）',
  `items`         text            COMMENT '逐题批改结果 JSON（题干分数/评语/错点/建议）',
  `error`         varchar(512)    NOT NULL DEFAULT '' COMMENT '失败原因，仅 status=failed/manual 有值',
  `requested_by`  bigint unsigned NOT NULL DEFAULT 0 COMMENT '发起批改的老师 user id',
  `created_at`    datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at`    datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`submission_id`),
  KEY `idx_task` (`task_id`),
  KEY `idx_assignment` (`assignment_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='主观题 AI 初评结果（老师采纳后才进成绩册）';
