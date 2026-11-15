-- Flyway V24：视频后处理（faststart）状态落库
--
-- 为什么要加这三列：改造前 course_videos 只有一个 faststarted 布尔，
-- 转码靠上传请求里起的裸 goroutine（fire-and-forget）：
--   1. 进程重启 / 部署 → goroutine 消失，那些视频永久停在"没处理"，没人再管；
--   2. 失败只在日志里，前端显示"上传成功"，用户只有拖不动进度条才发现；
--   3. 没有排队概念，一次传 10 个就同时跑 10 个 ffmpeg（每个都要整片下载 + 临时文件）。
--
-- 现在 course_videos 本身就是队列：worker 用条件更新认领 pending 行，
-- 重启后扫一遍就能续跑，失败原因和尝试次数都在表里。

ALTER TABLE `course_videos`
  ADD COLUMN `process_status`   VARCHAR(16)  NOT NULL DEFAULT 'pending' COMMENT '后处理状态：pending/processing/done/failed' AFTER `faststarted`,
  ADD COLUMN `process_error`    VARCHAR(512) NOT NULL DEFAULT ''        COMMENT '最后一次失败原因（成功或重排队时清空）'    AFTER `process_status`,
  ADD COLUMN `process_attempts` TINYINT UNSIGNED NOT NULL DEFAULT 0     COMMENT '已尝试次数，超过上限置 failed'             AFTER `process_error`,
  ADD COLUMN `processed_at`     DATETIME     DEFAULT NULL               COMMENT '状态最后变更时间，用于识别卡死的 processing' AFTER `process_attempts`;

-- worker 每轮按状态捞待处理行；不加这个索引就是全表扫。
ALTER TABLE `course_videos`
  ADD INDEX `idx_process_status` (`process_status`, `id`);

-- 存量数据回填：
--   已标记 faststarted 的算处理完；
--   其余全部排进队列 —— 顺带把历史上"goroutine 死在半路"的视频补处理掉。
UPDATE `course_videos` SET `process_status` = 'done', `processed_at` = NOW() WHERE `faststarted` = 1;
UPDATE `course_videos` SET `process_status` = 'pending' WHERE `faststarted` = 0;
