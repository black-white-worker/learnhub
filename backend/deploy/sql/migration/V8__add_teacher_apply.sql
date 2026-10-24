-- Flyway V8：users 表加老师申请标记
-- 学生点"申请老师"后 apply_teacher=1，管理员审批通过后 role=teacher

ALTER TABLE `users` ADD COLUMN `apply_teacher` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否申请成为老师' AFTER `role`;
