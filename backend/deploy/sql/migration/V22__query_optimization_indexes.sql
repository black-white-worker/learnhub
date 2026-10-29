-- Flyway V22：查询优化索引（B 端列表/聚合的"最左前缀 + 排序"补齐）
--
-- 原则：
--   1. 只给"已存在的高频 SQL"补索引，不为想象中的查询建索引；
--   2. 复合索引把 WHERE 等值列放前面、ORDER BY 列放后面，让分页不再 filesort；
--   3. 覆盖索引优先（video_progress），避免回表；
--   4. 幂等：线上可能已手工加过索引（热修复），先查 information_schema 再执行，
--      避免 Flyway 因 Duplicate key name 中断（写法同 V21）。

-- 1. class_invitations(class_id)
--    GetActiveInvitation / ListInvitations 都按 class_id 过滤；
--    原来只有 uk_code，查邀请码要全表扫。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'class_invitations' AND INDEX_NAME = 'idx_class');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `class_invitations` ADD INDEX `idx_class` (`class_id`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 2. exam_submissions(status, started_at)
--    自动收卷扫描 WHERE status='started'；V18 的 idx_started_at 单独一列用不上
--    （选择性太差），按状态前缀才能只扫"考试中"的行。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'exam_submissions' AND INDEX_NAME = 'idx_status_started');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `exam_submissions` ADD INDEX `idx_status_started` (`status`, `started_at`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 3. video_progress(video_id, completed, user_id)
--    成绩册"某班视频完成数"从班级课程 → 视频反查进度，需要按 video_id 驱动；
--    现有 uk_user_video(user_id, video_id) 的最左列是 user_id，帮不上。
--    三列都在索引里，查询可走覆盖索引，不回表。
--    注意：光有索引不够，SQL 里用 STRAIGHT_JOIN 固定"班级 → 课程视频 → 进度"的
--    连接顺序；否则优化器会全扫 video_progress（78ms），固定顺序后 5.7ms。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'video_progress' AND INDEX_NAME = 'idx_video_completed');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `video_progress` ADD INDEX `idx_video_completed` (`video_id`, `completed`, `user_id`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 4. assignments(class_id, created_at)
--    作业列表 WHERE class_id=? ORDER BY created_at DESC LIMIT ? ；
--    只按 class_id 过滤会先取出整班作业再 filesort。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'assignments' AND INDEX_NAME = 'idx_class_created');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `assignments` ADD INDEX `idx_class_created` (`class_id`, `created_at`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 5. exams(class_id, start_at)
--    考试列表 WHERE class_id=? ORDER BY start_at DESC LIMIT ? 。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'exams' AND INDEX_NAME = 'idx_class_start');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `exams` ADD INDEX `idx_class_start` (`class_id`, `start_at`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 6. announcements(class_id, is_top, created_at)
--    公告分页 ORDER BY is_top DESC, created_at DESC；三级索引可整体倒序扫描，
--    不用把整班公告取出来排序。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'announcements' AND INDEX_NAME = 'idx_class_top_created');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `announcements` ADD INDEX `idx_class_top_created` (`class_id`, `is_top`, `created_at`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;

-- 7. notifications(user_id, id)
--    消息中心 WHERE user_id=? ORDER BY id DESC LIMIT 50。
--    现有 idx_user_read(user_id, is_read) 无法直接满足 id 倒序，用户消息一多就 filesort。
SET @idx_exists := (SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notifications' AND INDEX_NAME = 'idx_user_id');
SET @ddl := IF(@idx_exists = 0,
  'ALTER TABLE `notifications` ADD INDEX `idx_user_id` (`user_id`, `id`)', 'DO 0');
PREPARE stmt FROM @ddl; EXECUTE stmt; DEALLOCATE PREPARE stmt;
