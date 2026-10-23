-- Flyway 版本化迁移 V4：把 V2 预置的两门示例课程创建者设为 admin
-- （V2 插入时没带 created_by，导致任何登录用户都能编辑）

UPDATE `courses` SET `created_by` = 'admin'
WHERE `created_by` = '' AND `name` IN ('Go 语言基础', '数据结构与算法');
