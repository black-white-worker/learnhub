-- MySQL 首次初始化时执行（数据卷已存在则不会重复执行）
-- learningplatform 由 MYSQL_DATABASE 自动创建，这里只补 videonest。
CREATE DATABASE IF NOT EXISTS videonest
  CHARACTER SET utf8mb4
  COLLATE utf8mb4_unicode_ci;

-- 说明：两个项目都使用 root 连接（与各自仓库里的 Flyway / 迁移配置保持一致）。
-- 如果后续要拆账号，在这里建 lp_user / vn_user，并同步改各自 .env.prod 里的 DB_USERNAME/DB_PASSWORD。
