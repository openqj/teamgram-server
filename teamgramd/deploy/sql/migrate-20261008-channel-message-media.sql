CREATE TABLE IF NOT EXISTS `apifull_channel_event` (
  `channel_id` BIGINT NOT NULL,
  `pts` INT NOT NULL,
  `pts_count` INT NOT NULL DEFAULT 1,
  `event_type` VARCHAR(16) NOT NULL,
  `message_ids` MEDIUMTEXT NOT NULL,
  `sender_user_id` BIGINT NOT NULL DEFAULT 0,
  `date` INT NOT NULL DEFAULT 0,
  `message` TEXT NOT NULL,
  `content_json` MEDIUMTEXT NULL,
  `edited_at` INT NOT NULL DEFAULT 0,
  `pinned` TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (`channel_id`, `pts`),
  KEY `idx_apifull_channel_event_cursor` (`channel_id`, `pts`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

SET @channel_message_content_column_exists = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'apifull_channel_message'
    AND column_name = 'content_json'
);
SET @channel_message_content_ddl = IF(
  @channel_message_content_column_exists = 0,
  'ALTER TABLE `apifull_channel_message` ADD COLUMN `content_json` MEDIUMTEXT NULL',
  'SELECT 1'
);
PREPARE channel_message_content_stmt FROM @channel_message_content_ddl;
EXECUTE channel_message_content_stmt;
DEALLOCATE PREPARE channel_message_content_stmt;

SET @channel_event_content_column_exists = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'apifull_channel_event'
    AND column_name = 'content_json'
);
SET @channel_event_content_ddl = IF(
  @channel_event_content_column_exists = 0,
  'ALTER TABLE `apifull_channel_event` ADD COLUMN `content_json` MEDIUMTEXT NULL',
  'SELECT 1'
);
PREPARE channel_event_content_stmt FROM @channel_event_content_ddl;
EXECUTE channel_event_content_stmt;
DEALLOCATE PREPARE channel_event_content_stmt;
