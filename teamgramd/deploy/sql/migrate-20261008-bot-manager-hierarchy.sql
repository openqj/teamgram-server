SET @bots_manager_column_exists = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'bots'
    AND column_name = 'manager_bot_id'
);
SET @bots_manager_column_ddl = IF(
  @bots_manager_column_exists = 0,
  'ALTER TABLE `bots` ADD COLUMN `manager_bot_id` bigint(20) NOT NULL DEFAULT ''0'' AFTER `creator_user_id`',
  'SELECT 1'
);
PREPARE bots_manager_column_stmt FROM @bots_manager_column_ddl;
EXECUTE bots_manager_column_stmt;
DEALLOCATE PREPARE bots_manager_column_stmt;

SET @bots_capability_column_exists = (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'bots'
    AND column_name = 'bot_can_manage_bots'
);
SET @bots_capability_column_ddl = IF(
  @bots_capability_column_exists = 0,
  'ALTER TABLE `bots` ADD COLUMN `bot_can_manage_bots` tinyint(1) NOT NULL DEFAULT ''0'' AFTER `manager_bot_id`',
  'SELECT 1'
);
PREPARE bots_capability_column_stmt FROM @bots_capability_column_ddl;
EXECUTE bots_capability_column_stmt;
DEALLOCATE PREPARE bots_capability_column_stmt;

SET @bots_manager_index_exists = (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'bots'
    AND index_name = 'idx_bots_manager_bot_id'
);
SET @bots_manager_index_ddl = IF(
  @bots_manager_index_exists = 0,
  'ALTER TABLE `bots` ADD KEY `idx_bots_manager_bot_id` (`manager_bot_id`,`bot_id`)',
  'SELECT 1'
);
PREPARE bots_manager_index_stmt FROM @bots_manager_index_ddl;
EXECUTE bots_manager_index_stmt;
DEALLOCATE PREPARE bots_manager_index_stmt;
