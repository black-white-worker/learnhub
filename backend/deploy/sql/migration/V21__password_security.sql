-- V21：强制修改初始密码 + 账号安全基线
--
-- 背景：V1 预置了 admin/123456，注释里写的是"教学版明文存储"。
-- 后来登录逻辑改成 bcrypt 并在登录成功后自动升级哈希，但初始密码本身
-- 从来没有被强制更换过 —— 部署到公网就是一个人人皆知的弱口令管理员。
--
-- 做法：给 users 加一个"必须改密"标记，登录后前端引导修改；
-- 改密成功后置 0。管理员重置他人密码时也会把它置 1。
--
-- 幂等写法：线上库可能已经手工执行过这条 ALTER（热修复），
-- 用 information_schema 判断后再执行，避免 Flyway 因 Duplicate column 中断。

SET @col_exists := (
  SELECT COUNT(*) FROM information_schema.COLUMNS
   WHERE TABLE_SCHEMA = DATABASE()
     AND TABLE_NAME = 'users'
     AND COLUMN_NAME = 'must_change_password'
);

SET @ddl := IF(
  @col_exists = 0,
  'ALTER TABLE `users` ADD COLUMN `must_change_password` tinyint(1) NOT NULL DEFAULT 0 COMMENT ''登录后必须先修改密码（初始账号/管理员重置后置 1）'' AFTER `role`',
  'DO 0'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

-- 存量 admin 账号标记为待改密（密码本身不变，避免把人锁在门外）。
-- 只在"本次刚加上列"时执行：重复执行不会把已经改过密码的 admin 再次标记。
SET @seed := IF(@col_exists = 0, 1, 0);
UPDATE `users` SET `must_change_password` = 1
 WHERE `username` = 'admin' AND @seed = 1;
