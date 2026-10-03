-- Persist all Layer 229 global privacy settings.
-- The procedure makes this migration safe to run against fresh and upgraded databases.

DROP PROCEDURE IF EXISTS ensure_user_global_privacy_column;
DELIMITER //
CREATE PROCEDURE ensure_user_global_privacy_column(
  IN p_column_name VARCHAR(64),
  IN p_definition VARCHAR(255)
)
BEGIN
  DECLARE column_count INT DEFAULT 0;

  SELECT COUNT(*) INTO column_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'user_global_privacy_settings'
    AND column_name = p_column_name;

  IF column_count = 0 THEN
    SET @global_privacy_ddl = CONCAT(
      'ALTER TABLE `user_global_privacy_settings` ADD COLUMN `',
      REPLACE(p_column_name, '`', '``'), '` ', p_definition
    );
    PREPARE global_privacy_stmt FROM @global_privacy_ddl;
    EXECUTE global_privacy_stmt;
    DEALLOCATE PREPARE global_privacy_stmt;
  END IF;
END//
DELIMITER ;

CALL ensure_user_global_privacy_column('keep_archived_unmuted', 'TINYINT(1) NOT NULL DEFAULT 0');
CALL ensure_user_global_privacy_column('keep_archived_folders', 'TINYINT(1) NOT NULL DEFAULT 0');
CALL ensure_user_global_privacy_column('hide_read_marks', 'TINYINT(1) NOT NULL DEFAULT 0');
CALL ensure_user_global_privacy_column('new_noncontact_peers_require_premium', 'TINYINT(1) NOT NULL DEFAULT 0');
CALL ensure_user_global_privacy_column('display_gifts_button', 'TINYINT(1) NOT NULL DEFAULT 0');
CALL ensure_user_global_privacy_column('noncontact_peers_paid_stars', 'BIGINT NULL');
CALL ensure_user_global_privacy_column('disallowed_gifts', 'TEXT');

DROP PROCEDURE ensure_user_global_privacy_column;
