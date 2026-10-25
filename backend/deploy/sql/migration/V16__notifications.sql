-- Flyway V16：站内消息通知
-- 覆盖两类消息：
-- 1) 管理员审批结果（学生申请老师后，审批通过/驳回都给被审批人写一条）
-- 2) 班级动态（公告 / 作业 / 讨论 / 签到发布时，给班级其他成员批量写一条）

CREATE TABLE IF NOT EXISTS `notifications` (
  `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id`    bigint unsigned NOT NULL COMMENT '接收人 user_id',
  `type`       varchar(32)     NOT NULL COMMENT 'approval/announcement/assignment/discussion/signin',
  `title`      varchar(256)    NOT NULL,
  `content`    varchar(1024)   NOT NULL DEFAULT '',
  `link_url`   varchar(256)    NOT NULL DEFAULT '' COMMENT '点击跳转的前端路由，如 /class/12',
  `is_read`    tinyint(1)      NOT NULL DEFAULT 0,
  `created_at` datetime        NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_read` (`user_id`, `is_read`),
  KEY `idx_user_time` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='站内消息';
