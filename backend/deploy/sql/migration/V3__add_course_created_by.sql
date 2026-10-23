-- Flyway 版本化迁移 V3：courses 表加创建者字段
-- 用于区分"课程创建者"和"浏览者"：创建者才能编辑/删除/上传，其他人只读。

ALTER TABLE `courses`
  ADD COLUMN `created_by` VARCHAR(64) NOT NULL DEFAULT '' COMMENT '创建者用户名' AFTER `teacher`;
