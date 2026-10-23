-- Flyway 版本化迁移 V2：课程模块元数据落库
-- 原课程模块是内存存储（进程重启即丢），这里建三张表持久化：
--   courses          课程基本信息 + 封面
--   course_videos    课程下的视频文件元信息（视频本体在 MinIO）
--   course_resources 课程下的文档资源（PPT/Word/PDF，本体在 MinIO）

-- 课程表
CREATE TABLE IF NOT EXISTS `courses` (
  `id`               bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '课程 id',
  `name`             varchar(128)    NOT NULL COMMENT '课程名称（唯一）',
  `intro`            varchar(512)    NOT NULL DEFAULT '' COMMENT '课程简介',
  `teacher`          varchar(64)     NOT NULL DEFAULT '' COMMENT '授课教师',
  `cover_url`        varchar(512)    NOT NULL DEFAULT '' COMMENT '封面原图 URL（MinIO）',
  `cover_thumb_url`  varchar(512)    NOT NULL DEFAULT '' COMMENT '封面缩略图 URL（MinIO）',
  `created_at`       datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程表';

-- 课程视频表
CREATE TABLE IF NOT EXISTS `course_videos` (
  `id`          bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '视频 id',
  `course_id`   bigint unsigned NOT NULL COMMENT '所属课程 id',
  `name`        varchar(255)    NOT NULL COMMENT '原始文件名',
  `url`         varchar(512)    NOT NULL COMMENT '视频播放 URL（MinIO）',
  `object_name` varchar(512)    NOT NULL DEFAULT '' COMMENT 'MinIO 对象路径（内部用）',
  `size`         bigint          NOT NULL DEFAULT 0 COMMENT '文件大小（字节）',
  `faststarted` tinyint(1)      NOT NULL DEFAULT 0 COMMENT '是否已生成 moov 前置版本',
  `uploaded_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '上传时间',
  PRIMARY KEY (`id`),
  KEY `idx_course` (`course_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程视频表';

-- 课程文档资源表（PPT / Word / PDF）
CREATE TABLE IF NOT EXISTS `course_resources` (
  `id`          bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '资源 id',
  `course_id`   bigint unsigned NOT NULL COMMENT '所属课程 id',
  `name`        varchar(255)    NOT NULL COMMENT '原始文件名',
  `url`         varchar(512)    NOT NULL COMMENT '资源访问 URL（MinIO）',
  `object_name` varchar(512)    NOT NULL DEFAULT '' COMMENT 'MinIO 对象路径（内部用）',
  `type`        varchar(16)     NOT NULL DEFAULT '' COMMENT '类型：pdf/ppt/doc',
  `size`        bigint          NOT NULL DEFAULT 0 COMMENT '文件大小（字节）',
  `uploaded_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '上传时间',
  PRIMARY KEY (`id`),
  KEY `idx_course` (`course_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='课程文档资源表';

-- 预置两门示例课程（和原内存版一致，INSERT IGNORE 保证可重复执行）
INSERT IGNORE INTO `courses` (`name`, `intro`, `teacher`) VALUES
  ('Go 语言基础', '从零上手 Go 语言，涵盖语法、并发与 Web 开发', '陈老师'),
  ('数据结构与算法', '链表、栈、队列、树与常见算法思想', '李老师');
