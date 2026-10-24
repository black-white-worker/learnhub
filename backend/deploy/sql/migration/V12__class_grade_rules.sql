-- Flyway V12：班级成绩权重配置

CREATE TABLE IF NOT EXISTS `class_grade_rules` (
  `class_id`        bigint unsigned NOT NULL,
  `video_weight`    int NOT NULL DEFAULT 30 COMMENT '视频完课权重%',
  `assignment_weight` int NOT NULL DEFAULT 50 COMMENT '作业权重%',
  `signin_weight`   int NOT NULL DEFAULT 10 COMMENT '签到权重%',
  `exam_weight`     int NOT NULL DEFAULT 10 COMMENT '考试权重%',
  `updated_at`      datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`class_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='班级成绩权重';
