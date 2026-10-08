DROP PROCEDURE IF EXISTS apifull_ensure_group_call_media_source;
DELIMITER //
CREATE PROCEDURE apifull_ensure_group_call_media_source()
BEGIN
  DECLARE column_count INT DEFAULT 0;
  DECLARE index_count INT DEFAULT 0;

  SELECT COUNT(*) INTO column_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'apifull_group_call_participant'
    AND column_name = 'media_source';

  IF column_count = 0 THEN
    ALTER TABLE apifull_group_call_participant
      ADD COLUMN media_source INT NULL AFTER user_id;
  END IF;

  SELECT COUNT(*) INTO index_count
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = 'apifull_group_call_participant'
    AND index_name = 'uniq_apifull_group_call_participant_source';

  IF index_count = 0 THEN
    SET @group_call_media_index_ddl =
      'CREATE UNIQUE INDEX uniq_apifull_group_call_participant_source ON apifull_group_call_participant (call_id, media_source)';
    PREPARE group_call_media_index_stmt FROM @group_call_media_index_ddl;
    EXECUTE group_call_media_index_stmt;
    DEALLOCATE PREPARE group_call_media_index_stmt;
  END IF;
END//
DELIMITER ;

CALL apifull_ensure_group_call_media_source();
DROP PROCEDURE apifull_ensure_group_call_media_source;
