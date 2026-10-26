-- V19：考试/作业出题器改造（对标腾讯问卷）
-- 1) exam_questions 增加 order_num / required / multiple / fill / fileupload 题型
-- 2) exams 增加 allow_late / tab_limit / duration_minutes 已有
-- 3) assignment_questions 同步扩展题型和字段
-- 4) assignments 增加 allow_late / late_deadline

ALTER TABLE `exam_questions`
  ADD COLUMN `order_num` int NOT NULL DEFAULT 0 COMMENT '排序号，越小越靠前' AFTER `score`,
  ADD COLUMN `required` tinyint NOT NULL DEFAULT 1 COMMENT '是否必答 1必答0选答' AFTER `order_num`,
  ADD COLUMN `created_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP AFTER `required`;

ALTER TABLE `exams`
  ADD COLUMN `allow_late` tinyint NOT NULL DEFAULT 0 COMMENT '是否允许迟到补交' AFTER `total_score`,
  ADD COLUMN `tab_limit` int NOT NULL DEFAULT 0 COMMENT '切屏次数限制，0不限制' AFTER `allow_late`;

ALTER TABLE `assignment_questions`
  ADD COLUMN `order_num` int NOT NULL DEFAULT 0 COMMENT '排序号' AFTER `score`,
  ADD COLUMN `required` tinyint NOT NULL DEFAULT 1 COMMENT '是否必答' AFTER `order_num`;

ALTER TABLE `assignments`
  ADD COLUMN `allow_late` tinyint NOT NULL DEFAULT 0 COMMENT '是否允许补交' AFTER `total_score`,
  ADD COLUMN `late_deadline` datetime DEFAULT NULL COMMENT '补交截止时间' AFTER `allow_late`;

-- 给已有题目按 id 顺序赋初始 order_num
UPDATE `exam_questions` SET `order_num` = `id`;
UPDATE `assignment_questions` SET `order_num` = `id`;
