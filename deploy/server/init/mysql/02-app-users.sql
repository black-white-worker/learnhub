-- =====================================================================
-- 分项目数据库账号（替代"三个项目共用 root"）
--
-- 【重要】这个文件只在 mysql_data 卷第一次初始化时自动执行。
-- 卷已经存在（服务器现在就是）→ 改这里不生效，必须手动跑一次：
--   docker compose -f docker-compose.infra.yml --env-file .env exec -T mysql \
--     mysql -uroot -p < deploy/server/init/mysql/02-app-users.sql
--   # 交互式输入 root 密码；或用 MYSQL_PWD 传，别把密码写进命令行历史
-- 语句是幂等的（IF NOT EXISTS + 覆盖式 GRANT），重复执行没有副作用。
--
-- 【为什么值得拆】现在 learningplatform 和 videonest 都用 root 连同一个 mysqld。
-- root 有全局权限，任何一方被 SQL 注入或凭据泄露，拿到的是整个实例：
-- 两个库都能读、能 DROP，还能读 mysql.user 里的哈希。
-- 拆完之后 lp 的泄露只影响 lp 自己的库，DDL 权限也只剩 Flyway 部署时需要。
--
-- 【切换顺序】账号建好 ≠ 生效。要按下面三步走，缺一步就是启动报错：
--   1) 本文件在服务器上执行过，用 root 连进去 SHOW GRANTS FOR 'lp_user'@'%' 确认；
--   2) 改各自的 .env.prod：DB_USERNAME / DB_PASSWORD（或 lp 的 MYSQL_DSN 里的用户段）；
--   3) 只重启应用层，不动中间件：
--        cd /opt/apps/videonest && docker compose -f docker-compose.server.yml \
--          --env-file .env.prod up -d --no-deps vn-backend
--      健康检查通过再切下一个项目。
--
-- 【两个已知的坑】
--   - learningplatform 的 flyway 服务在 docker-compose.server.yml 里写死了
--     -user=root -password=${MYSQL_ROOT_PASSWORD}，账号拆分对它无效；
--     要用 lp_user 得先改成变量插值，并且该账号需要 videonest/learningplatform
--     两个库各自的 DDL 权限。这一步风险最高，建议最后做或干脆不做。
--   - videonest 的 Flyway 迁移要建索引、改表，需要 DDL；GRANT 里已包含 ALTER/INDEX，
--     但 CREATE ROUTINE/EVENT 没有——如果以后迁移里出现存储过程或定时任务，
--     这里要补权限，否则迁移会在半路失败（届时用备份还原，见 redeploy.sh）。
-- =====================================================================

-- 两个账号的密码从环境变量取（官方 mysql 镜像的 entrypoint 会 source 本目录的 .sh，
-- 但 .sql 文件不做变量替换），所以这里是写死的占位密码。
-- 落库前请务必改成随机值：openssl rand -hex 24
CREATE USER IF NOT EXISTS 'lp_user'@'%' IDENTIFIED BY 'CHANGE_ME_lp';
CREATE USER IF NOT EXISTS 'vn_user'@'%' IDENTIFIED BY 'CHANGE_ME_vn';

-- 只管自己那一个库：库级授权，不给自己库以外的任何对象
GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER,
      CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, SHOW VIEW, EVENT, TRIGGER
  ON `learningplatform`.* TO 'lp_user'@'%';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, REFERENCES, INDEX, ALTER,
      CREATE TEMPORARY TABLES, LOCK TABLES, EXECUTE, SHOW VIEW, EVENT, TRIGGER
  ON `videonest`.* TO 'vn_user'@'%';

FLUSH PRIVILEGES;

-- 验证（手动执行，不属于初始化流程）：
--   docker exec -i infra-mysql mysql -uvn_user -p -e "show databases; select current_user();"
--   预期：只列出 information_schema / performance_schema / videonest
