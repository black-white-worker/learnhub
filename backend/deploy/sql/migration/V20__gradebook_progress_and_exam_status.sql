-- V20：成绩册 + 学习进度 + 考试交卷状态机
--
-- 本次改造的三个数据基座（对标成熟平台的做法）：
--   1. grade_items / grade_scores：Canvas 式成绩册。分数在提交/批改时落库，
--      查询时只做聚合，不再用子查询现算（旧实现在 Go 里把视频项写死 100 分）。
--   2. video_progress：Open edX 式学习进度。按 user 维度记录观看时长，
--      完成度按"去重后的累计观看秒数"算，而不是"播到第几秒"。
--   3. exam_submissions.status：交卷状态机（started → submitted / expired），
--      用于锁死重复交卷（旧实现是裸 Upsert，可以交完看答案再交一次）。

-- ===== 1. 成绩册：成绩项与成绩记录 =====

CREATE TABLE IF NOT EXISTS `grade_items` (
  `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`    bigint unsigned NOT NULL COMMENT '所属班级',
  `source_type` varchar(16)     NOT NULL COMMENT '来源类型：assignment / exam',
  `source_id`   bigint unsigned NOT NULL COMMENT '来源 id（作业 id 或考试 id）',
  `title`       varchar(256)    NOT NULL DEFAULT '' COMMENT '成绩项名称',
  `max_score`   decimal(6,2)    NOT NULL DEFAULT 100 COMMENT '满分',
  `created_at`  datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_source` (`source_type`, `source_id`),
  KEY `idx_class` (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='成绩项（每个作业/考试一条）';

CREATE TABLE IF NOT EXISTS `grade_scores` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `item_id`    bigint unsigned NOT NULL COMMENT '成绩项 id',
  `user_id`    bigint unsigned NOT NULL COMMENT '学生 id',
  `score`      decimal(6,2)    NOT NULL DEFAULT 0 COMMENT '得分',
  `graded_by`  bigint unsigned NOT NULL DEFAULT 0 COMMENT '批改人 id（0=系统自动判分）',
  `graded_at`  datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_item_user` (`item_id`, `user_id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='成绩记录（每个学生每个成绩项一条）';

-- ===== 2. 学习进度：按用户记录视频观看 =====

CREATE TABLE IF NOT EXISTS `video_progress` (
  `id`               bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id`          bigint unsigned NOT NULL,
  `course_id`        bigint unsigned NOT NULL,
  `video_id`         bigint unsigned NOT NULL,
  `position_seconds` int             NOT NULL DEFAULT 0 COMMENT '上次播放位置（秒），用于续播',
  `watched_seconds`  int             NOT NULL DEFAULT 0 COMMENT '累计有效观看秒数（前端上报的增量累加）',
  `duration_seconds` int             NOT NULL DEFAULT 0 COMMENT '视频总时长（秒），由播放器上报',
  `completed`        tinyint         NOT NULL DEFAULT 0 COMMENT '是否看完（累计观看 >= 时长的 90%）',
  `updated_at`       datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_video` (`user_id`, `video_id`),
  KEY `idx_user_course` (`user_id`, `course_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户视频学习进度';

-- ===== 3. 考试交卷状态机 =====

ALTER TABLE `exam_submissions`
  ADD COLUMN `status`     varchar(16)     NOT NULL DEFAULT 'started' COMMENT 'started=考试中 submitted=已交卷 expired=超时强制交卷' AFTER `answers`,
  ADD COLUMN `graded_by`  bigint unsigned NOT NULL DEFAULT 0 COMMENT '批改老师 id' AFTER `teacher_score`,
  ADD COLUMN `graded_at`  datetime        DEFAULT NULL COMMENT '批改时间' AFTER `graded_by`,
  ADD COLUMN `violations` int             NOT NULL DEFAULT 0 COMMENT '切屏/违纪次数（前端上报累加）' AFTER `graded_at`,
  ADD KEY `idx_user` (`user_id`);

-- 存量数据回填：已经写入了作答内容的视为"已交卷"（旧版本没有 status 字段）
UPDATE `exam_submissions` SET `status` = 'submitted' WHERE `answers` IS NOT NULL AND `answers` <> '';

-- 考试补一个补交截止时间，与 assignments.late_deadline 对齐（allow_late 之前只有开关没有时间）
ALTER TABLE `exams`
  ADD COLUMN `late_deadline` datetime DEFAULT NULL COMMENT '允许补交的截止时间（allow_late=1 时生效）' AFTER `allow_late`;
