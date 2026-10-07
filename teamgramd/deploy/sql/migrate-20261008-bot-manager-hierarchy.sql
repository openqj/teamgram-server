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

CREATE TABLE IF NOT EXISTS `bot_manager_capability_audit` (
  `id` bigint(20) NOT NULL AUTO_INCREMENT,
  `bot_id` bigint(20) NOT NULL,
  `old_enabled` tinyint(1) NOT NULL,
  `new_enabled` tinyint(1) NOT NULL,
  `operator_db_user` varchar(288) COLLATE utf8mb4_unicode_ci NOT NULL,
  `reason` varchar(512) COLLATE utf8mb4_unicode_ci NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_bot_manager_capability_audit_bot` (`bot_id`,`id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
