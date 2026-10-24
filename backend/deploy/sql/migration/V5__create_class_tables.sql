-- Flyway V5：班级模块基础表
-- classes：班级
-- class_members：班级成员（学生/老师）
-- class_courses：班级绑定的课程
-- class_invitations：加入班级的邀请码

CREATE TABLE IF NOT EXISTS `classes` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `name`       varchar(128)    NOT NULL COMMENT '班级名，如：计科2401班',
  `grade`      varchar(32)     NOT NULL DEFAULT '' COMMENT '年级，如：2024级',
  `major`      varchar(128)    NOT NULL DEFAULT '' COMMENT '专业',
  `head_teacher_id` bigint unsigned NOT NULL DEFAULT 0 COMMENT '班主任 user_id',
  `created_by` varchar(64)     NOT NULL DEFAULT '' COMMENT '创建者用户名',
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级表';

CREATE TABLE IF NOT EXISTS `class_members` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL COMMENT '班级id',
  `user_id`    bigint unsigned NOT NULL COMMENT '用户id',
  `role`       varchar(16)     NOT NULL DEFAULT 'student' COMMENT 'teacher/student',
  `joined_at`  datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_class_user` (`class_id`, `user_id`),
  KEY `idx_user` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级成员';

CREATE TABLE IF NOT EXISTS `class_courses` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL,
  `course_id`  bigint unsigned NOT NULL,
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_class_course` (`class_id`, `course_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级-课程绑定';

CREATE TABLE IF NOT EXISTS `class_invitations` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `class_id`   bigint unsigned NOT NULL,
  `code`       varchar(16)     NOT NULL COMMENT '6位邀请码',
  `expires_at` datetime        NULL COMMENT '过期时间，NULL=永不过期',
  `max_uses`   int             NOT NULL DEFAULT 0 COMMENT '0=不限次数',
  `used_count` int             NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级邀请码';
