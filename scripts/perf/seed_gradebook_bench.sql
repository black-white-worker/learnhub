-- 本地/CI 用的成绩册合成数据（只灌临时库，绝不导入业务库）。
--
-- 用途：验证 model/grade.go 的 ListGradebookRaw 改写前后"结果一致 + 延迟下降"。
-- 不是 Flyway 迁移：文件名不在 migration/ 下，Flyway 不会执行。
--
-- 用法（PowerShell / bash 同理）：
--   mysql -e "DROP DATABASE IF EXISTS lp_perf; CREATE DATABASE lp_perf CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"
--   for f in backend/deploy/sql/migration/V*.sql; do mysql lp_perf < "$f"; done     # 建表 + V22 索引
--   mysql lp_perf < scripts/perf/seed_gradebook_bench.sql
--   # 然后把 model/grade.go 里的 ListGradebookRaw SQL（classId=9001）拿去 EXPLAIN ANALYZE
--
-- 规模：1 个班 120 名学生、30 份作业 + 8 场考试（4560 条成绩）、40 次签到
--       （3840 条记录）、4 门课 48 个视频（3456 条进度）、20 条公告（1440 条已读）、
--       12000 条通知、960 份答卷（其中 5% 处于考试中）。
-- 说明：这是"量级验证"数据，不是生产数据分布；结论只用于判断改写有没有数量级退化。

SET SESSION cte_max_recursion_depth = 5000;

DROP TABLE IF EXISTS nums;
CREATE TABLE nums (n INT PRIMARY KEY);
INSERT INTO nums (n)
WITH RECURSIVE seq(n) AS (
  SELECT 1 UNION ALL SELECT n + 1 FROM seq WHERE n < 500
)
SELECT n FROM seq;

-- 用户与班级成员
INSERT INTO users (id, username, password, nickname, role)
SELECT 1000 + n, CONCAT('stu', n), 'x', CONCAT('学生', n), 'student' FROM nums WHERE n <= 120;
INSERT INTO users (id, username, password, nickname, role) VALUES (900, 'teacher', 'x', '老师', 'teacher');

INSERT INTO classes (id, name, grade, major, head_teacher_id, created_by)
VALUES (9001, '性能验证班', '2026', '计算机', 900, 'teacher');

INSERT INTO class_members (class_id, user_id, role)
SELECT 9001, 1000 + n, 'student' FROM nums WHERE n <= 120;
INSERT INTO class_members (class_id, user_id, role) VALUES (9001, 900, 'teacher');

-- 课程 + 视频 + 绑定
INSERT INTO courses (id, name, intro, teacher, created_by)
SELECT 9000 + n, CONCAT('课程', n), '', 'teacher', 'teacher' FROM nums WHERE n <= 4;
INSERT INTO course_videos (id, course_id, name, url, object_name, size)
SELECT 9000 + (c.n - 1) * 12 + v.n,
       9000 + c.n,
       CONCAT('视频', c.n, '-', v.n),
       '/course/v.mp4', CONCAT('videos/', c.n, '-', v.n, '.mp4'), 1000
FROM nums c JOIN nums v ON v.n <= 12 WHERE c.n <= 4;
INSERT INTO class_courses (class_id, course_id)
SELECT 9001, 9000 + n FROM nums WHERE n <= 4;

-- 作业 + 考试 + 成绩项
INSERT INTO assignments (id, class_id, course_id, title, description, deadline, total_score, publisher_id)
SELECT 9000 + n, 9001, 9001, CONCAT('作业', n), '', NOW(), 100, 900 FROM nums WHERE n <= 30;
INSERT INTO exams (id, class_id, title, description, start_at, end_at, duration_minutes, total_score, created_by)
SELECT 9000 + n, 9001, CONCAT('考试', n), '', NOW() - INTERVAL 1 DAY, NOW(), 60, 100, 900 FROM nums WHERE n <= 8;

INSERT INTO grade_items (id, class_id, source_type, source_id, title, max_score)
SELECT 9000 + n, 9001, 'assignment', 9000 + n, CONCAT('作业', n), 100 FROM nums WHERE n <= 30;
INSERT INTO grade_items (id, class_id, source_type, source_id, title, max_score)
SELECT 9100 + n, 9001, 'exam', 9000 + n, CONCAT('考试', n), 100 FROM nums WHERE n <= 8;

-- 成绩：120 学生 × 38 项
INSERT INTO grade_scores (item_id, user_id, score, graded_by)
SELECT gi.id, 1000 + s.n, (s.n * 7 + gi.id * 3) % 101, 0
FROM nums s JOIN grade_items gi ON gi.class_id = 9001
WHERE s.n <= 120;

-- 签到：40 场，每场约 80% 学生签到
INSERT INTO class_signin (id, class_id, title, start_at, teacher_id, code, expires_at)
SELECT 9000 + n, 9001, CONCAT('签到', n), NOW() - INTERVAL n DAY, 900, LPAD(n, 6, '0'), NOW() - INTERVAL n DAY + INTERVAL 10 MINUTE
FROM nums WHERE n <= 40;
INSERT INTO signin_records (signin_id, user_id, signed_at)
SELECT 9000 + k.n, 1000 + s.n, NOW() - INTERVAL k.n DAY
FROM nums k JOIN nums s ON s.n <= 120
WHERE k.n <= 40 AND (s.n + k.n) % 5 <> 0;

-- 视频进度：约 60% 视频看完
INSERT INTO video_progress (user_id, course_id, video_id, position_seconds, watched_seconds, duration_seconds, completed)
SELECT 1000 + s.n, cv.course_id, cv.id, 600, 600, 600, 1
FROM nums s JOIN course_videos cv ON cv.id BETWEEN 9001 AND 9048
WHERE s.n <= 120 AND (s.n + cv.id) % 5 < 3;

-- 公告 + 已读（20 条 × 约 60% 已读）
INSERT INTO announcements (id, class_id, title, content, publisher_id, is_top, created_at)
SELECT 9000 + n, 9001, CONCAT('公告', n), '内容', 900, 0, NOW() - INTERVAL n HOUR
FROM nums WHERE n <= 20;
INSERT INTO announcement_reads (announcement_id, user_id, read_at)
SELECT 9000 + a.n, 1000 + s.n, NOW()
FROM nums a JOIN nums s ON s.n <= 120
WHERE a.n <= 20 AND (s.n + a.n) % 5 < 3;

-- 通知：每人 100 条，便于验证列表分页/未读数
INSERT INTO notifications (user_id, type, title, content, link_url, is_read, created_at)
SELECT 1000 + s.n, 'system', CONCAT('通知', m.n), '', '/x', (m.n % 2 = 0), NOW() - INTERVAL m.n HOUR
FROM nums s JOIN nums m ON m.n <= 100
WHERE s.n <= 120;

-- 答卷：8 场 × 120 人，约 5% 处于考试中（生产里"进行中"永远是少数），其余已交
INSERT INTO exam_submissions (exam_id, user_id, answers, status, started_at, submitted_at)
SELECT 9000 + e.n, 1000 + s.n, '{}',
       IF((s.n + e.n) % 20 = 0, 'started', 'submitted'),
       NOW() - INTERVAL 2 HOUR,
       NOW() - INTERVAL 1 HOUR
FROM nums e JOIN nums s ON s.n <= 120
WHERE e.n <= 8;

INSERT INTO exam_questions (exam_id, type, stem, options, answer, score, order_num)
SELECT 9000 + e.n, 'single', CONCAT('题', q.n), '["A","B"]', 'A', 10, q.n
FROM nums e JOIN nums q ON q.n <= 5
WHERE e.n <= 8;

SELECT
  (SELECT COUNT(*) FROM class_members WHERE class_id = 9001) AS members,
  (SELECT COUNT(*) FROM grade_scores) AS scores,
  (SELECT COUNT(*) FROM signin_records) AS signins,
  (SELECT COUNT(*) FROM video_progress) AS progress,
  (SELECT COUNT(*) FROM announcement_reads) AS read_rows,
  (SELECT COUNT(*) FROM notifications) AS notifications,
  (SELECT COUNT(*) FROM exam_submissions) AS submissions;
