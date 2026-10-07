SET @bots_creator_index_exists = (
  SELECT COUNT(*) FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'bots'
    AND index_name = 'idx_bots_creator_bot_id'
);
SET @bots_creator_index_ddl = IF(
  @bots_creator_index_exists = 0,
  'ALTER TABLE `bots` ADD KEY `idx_bots_creator_bot_id` (`creator_user_id`,`bot_id`)',
  'SELECT 1'
);
PREPARE bots_creator_index_stmt FROM @bots_creator_index_ddl;
EXECUTE bots_creator_index_stmt;
DEALLOCATE PREPARE bots_creator_index_stmt;
