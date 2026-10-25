-- Flyway V17：签到事件化
-- 1) class_signin 增加 title（签到事件标题）和 start_at（开始时间），
--    使其成为有时间范围的事件任务，而不是只生成一个签到码。
-- 2) exam_questions / assignment_questions 的 type 注释补充 multiple（多选题）。
--    type 是 varchar，无需改列类型，只改注释。

ALTER TABLE `class_signin`
  ADD COLUMN `title`     varchar(128) NOT NULL DEFAULT '课堂签到' AFTER `class_id`,
  ADD COLUMN `start_at`  datetime     NOT NULL DEFAULT CURRENT_TIMESTAMP AFTER `title`;

ALTER TABLE `exam_questions`
  MODIFY COLUMN `type` varchar(16) NOT NULL COMMENT 'single=单选, multiple=多选, judge=判断, essay=主观';

ALTER TABLE `assignment_questions`
  MODIFY COLUMN `type` varchar(16) NOT NULL COMMENT 'single=单选, multiple=多选, judge=判断, essay=主观';
