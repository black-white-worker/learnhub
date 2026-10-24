-- Flyway V6：users 表加 role 字段，支持三级权限
-- admin / teacher / student，注册默认 student

ALTER TABLE `users` ADD COLUMN `role` varchar(16) NOT NULL DEFAULT 'student' COMMENT 'admin/teacher/student' AFTER `nickname`;

-- 预置的 admin 账号改成 admin 角色
UPDATE `users` SET `role` = 'admin' WHERE `username` = 'admin';
