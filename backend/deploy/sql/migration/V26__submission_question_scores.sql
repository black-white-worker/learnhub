-- Flyway V26：老师逐题最终评分
--
-- 为什么要存这一列：
--   作业/考试的主观题批改从「一个总分输入框」改成「每道主观题各自给分，再累加」。
--   逐题分只存在前端的话，老师下次打开同一份答卷，面板只能重新按 AI 初评分填充，
--   既会把老师改过的分数冲掉，也会出现「逐题分加起来 ≠ 已保存总分」的对不上账。
--
-- 存 JSON 而不是单开一张表：
--   逐题分只服务于「重新打开时回填」，没有按题检索/统计的需求；
--   一次批改就是这一份答卷的一组数，跟 score / comment 同生命周期，没必要多一张表。
--
-- 空串表示「这份答卷没有逐题评分」（老数据 / 老师只敲了总分），前端按 AI 初评回填。
ALTER TABLE `assignment_submissions`
  ADD COLUMN `question_scores` varchar(2048) NOT NULL DEFAULT ''
    COMMENT '老师逐题最终评分 JSON，如 {"12":6,"13":8}' AFTER `comment`;

ALTER TABLE `exam_submissions`
  ADD COLUMN `question_scores` varchar(2048) NOT NULL DEFAULT ''
    COMMENT '老师逐题最终评分 JSON，如 {"21":8}' AFTER `comment`;
