-- APIFull tables and columns required by the readonly production runtime.
-- This file is idempotent so it can be used for a fresh database or a
-- provisioned database that was created by an earlier APIFull migration.

CREATE TABLE IF NOT EXISTS apifull_channel_admin_log (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  channel_id BIGINT NOT NULL,
  actor_user_id BIGINT NOT NULL,
  target_user_id BIGINT NOT NULL DEFAULT 0,
  action VARCHAR(64) NOT NULL,
  prev_member MEDIUMTEXT NOT NULL,
  new_member MEDIUMTEXT NOT NULL,
  date INT NOT NULL,
  KEY idx_apifull_channel_admin_log_channel (channel_id, id),
  KEY idx_apifull_channel_admin_log_action (channel_id, action, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_stars_offer (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  kind VARCHAR(16) NOT NULL,
  stars BIGINT NOT NULL,
  store_product VARCHAR(191) NOT NULL DEFAULT '',
  currency VARCHAR(16) NOT NULL DEFAULT '',
  amount BIGINT NOT NULL DEFAULT 0,
  extended TINYINT NOT NULL DEFAULT 0,
  active TINYINT NOT NULL DEFAULT 1,
  UNIQUE KEY uniq_apifull_stars_offer (kind, stars, store_product, currency, amount),
  KEY idx_apifull_stars_offer_active (kind, active, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_payment_request (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  request_key VARCHAR(191) NOT NULL,
  provider VARCHAR(32) NOT NULL,
  fingerprint CHAR(64) NOT NULL,
  state VARCHAR(16) NOT NULL,
  transaction_id VARCHAR(191) NOT NULL DEFAULT '',
  currency VARCHAR(16) NOT NULL DEFAULT '',
  amount BIGINT NOT NULL DEFAULT 0,
  peer_id BIGINT NOT NULL DEFAULT 0,
  msg_id INT NOT NULL DEFAULT 0,
  error_text VARCHAR(255) NOT NULL DEFAULT '',
  created_at INT NOT NULL,
  updated_at INT NOT NULL,
  UNIQUE KEY uniq_apifull_payment_request (user_id, request_key),
  KEY idx_apifull_payment_request_state (state, updated_at),
  KEY idx_apifull_payment_request_transaction (provider, transaction_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_payment_ledger (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  request_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  state VARCHAR(16) NOT NULL,
  provider VARCHAR(32) NOT NULL,
  transaction_id VARCHAR(191) NOT NULL DEFAULT '',
  currency VARCHAR(16) NOT NULL DEFAULT '',
  amount BIGINT NOT NULL DEFAULT 0,
  receipt_hash CHAR(64) NOT NULL DEFAULT '',
  created_at INT NOT NULL,
  UNIQUE KEY uniq_apifull_payment_ledger_state (request_id, state),
  KEY idx_apifull_payment_ledger_user (user_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_payment_receipt (
  request_id BIGINT NOT NULL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  provider VARCHAR(32) COLLATE utf8mb4_bin NOT NULL,
  transaction_id VARCHAR(191) COLLATE utf8mb4_bin NOT NULL,
  currency VARCHAR(16) NOT NULL DEFAULT '',
  amount BIGINT NOT NULL DEFAULT 0,
  peer_id BIGINT NOT NULL DEFAULT 0,
  msg_id INT NOT NULL DEFAULT 0,
  title VARCHAR(255) NOT NULL DEFAULT '',
  receipt MEDIUMBLOB NOT NULL,
  created_at INT NOT NULL,
  UNIQUE KEY uniq_apifull_payment_receipt_transaction (provider, transaction_id),
  KEY idx_apifull_payment_receipt_message (user_id, peer_id, msg_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_payment_entitlement_outbox (
  request_id BIGINT NOT NULL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  provider VARCHAR(32) COLLATE utf8mb4_bin NOT NULL,
  transaction_id VARCHAR(191) COLLATE utf8mb4_bin NOT NULL,
  months TINYINT UNSIGNED NOT NULL,
  state VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE KEY uniq_apifull_payment_entitlement_transaction (provider, transaction_id),
  KEY idx_apifull_payment_entitlement_due (state, next_attempt_at, request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;


CREATE TABLE IF NOT EXISTS apifull_channel_event (
  channel_id BIGINT NOT NULL,
  pts INT NOT NULL,
  pts_count INT NOT NULL DEFAULT 1,
  event_type VARCHAR(16) NOT NULL,
  message_ids MEDIUMTEXT NOT NULL,
  sender_user_id BIGINT NOT NULL DEFAULT 0,
  date INT NOT NULL DEFAULT 0,
  message TEXT NOT NULL,
  content_json MEDIUMTEXT NULL,
  edited_at INT NOT NULL DEFAULT 0,
  pinned TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, pts),
  KEY idx_apifull_channel_event_cursor (channel_id, pts)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_message_content_read (
  user_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  read_at INT NOT NULL,
  PRIMARY KEY (user_id, channel_id, message_id),
  KEY idx_apifull_channel_content_read_message (channel_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_message_request (
  channel_id BIGINT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  random_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  pts INT NOT NULL,
  request_hash BINARY(32) NOT NULL,
  created_at INT NOT NULL,
  PRIMARY KEY (channel_id, sender_user_id, random_id),
  KEY idx_apifull_channel_message_request_message (channel_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_delivery_outbox (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  channel_id BIGINT NOT NULL,
  pts_from INT NOT NULL,
  pts_to INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  exclude_auth_key_id BIGINT NOT NULL DEFAULT 0,
  state VARCHAR(16) NOT NULL DEFAULT 'pending',
  payload MEDIUMBLOB NOT NULL,
  created_at BIGINT NOT NULL,
  UNIQUE KEY uniq_apifull_channel_delivery_event (channel_id, pts_from, pts_to),
  KEY idx_apifull_channel_delivery_channel (channel_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_delivery_recipient (
  delivery_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  state VARCHAR(16) NOT NULL DEFAULT 'pending',
  attempts INT NOT NULL DEFAULT 0,
  next_attempt_at BIGINT NOT NULL,
  delivered_at BIGINT NOT NULL DEFAULT 0,
  last_error VARCHAR(255) NOT NULL DEFAULT '',
  PRIMARY KEY (delivery_id, user_id),
  KEY idx_apifull_channel_delivery_due (state, next_attempt_at, delivery_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP PROCEDURE IF EXISTS apifull_ensure_latest_column;
DELIMITER //
CREATE PROCEDURE apifull_ensure_latest_column(
  IN p_table_name VARCHAR(64),
  IN p_column_name VARCHAR(64),
  IN p_definition VARCHAR(255)
)
BEGIN
  DECLARE column_count INT DEFAULT 0;

  SELECT COUNT(*) INTO column_count
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = p_table_name
    AND column_name = p_column_name;

  IF column_count = 0 THEN
    SET @apifull_latest_ddl = CONCAT(
      'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
      '` ADD COLUMN `', REPLACE(p_column_name, '`', '``'), '` ', p_definition
    );
    PREPARE apifull_latest_stmt FROM @apifull_latest_ddl;
    EXECUTE apifull_latest_stmt;
    DEALLOCATE PREPARE apifull_latest_stmt;
  END IF;
END//
DELIMITER ;

CALL apifull_ensure_latest_column('apifull_channel', 'signatures_enabled', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'signature_profiles_enabled', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'antispam', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'hidden_prehistory', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'participants_hidden', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'slowmode_seconds', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'color', 'INT NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'background_emoji_id', 'BIGINT NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'profile_color', 'INT NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'profile_background_emoji_id', 'BIGINT NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'photo_id', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'photo_dc_id', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'photo_has_video', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel', 'location_lat', 'DOUBLE NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'location_long', 'DOUBLE NULL');
CALL apifull_ensure_latest_column('apifull_channel', 'location_address', CONCAT('VARCHAR(255) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_channel', 'username', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_channel', 'discussion_group_id', 'BIGINT NULL');
CALL apifull_ensure_latest_column('apifull_channel_member', 'admin_rights', CONCAT('VARCHAR(2048) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_channel_member', 'admin_rank', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_channel_member', 'banned_rights', CONCAT('VARCHAR(2048) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_channel_member', 'banned_by_user_id', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel_member', 'banned_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel_message', 'pinned', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel_message', 'reply_to_msg_id', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel_message', 'reply_to_top_id', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_channel_message', 'content_json', 'MEDIUMTEXT NULL');
CALL apifull_ensure_latest_column('apifull_channel_event', 'content_json', 'MEDIUMTEXT NULL');
CALL apifull_ensure_latest_column('apifull_group_call', 'title', CONCAT('VARCHAR(255) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_latest_column('apifull_group_call', 'rtmp_stream', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_group_call', 'conference', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_group_call', 'schedule_date', 'INT NULL');
CALL apifull_ensure_latest_column('apifull_group_call_participant', 'presentation_active', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_group_call_participant', 'join_params', 'MEDIUMTEXT NOT NULL DEFAULT ('''')');
CALL apifull_ensure_latest_column('apifull_call', 'ga_hash', 'VARBINARY(256) NULL');
CALL apifull_ensure_latest_column('apifull_call', 'gb', 'VARBINARY(256) NULL');
CALL apifull_ensure_latest_column('apifull_call', 'ga', 'VARBINARY(256) NULL');
CALL apifull_ensure_latest_column('apifull_call', 'protocol', 'MEDIUMTEXT NULL');
CALL apifull_ensure_latest_column('apifull_call', 'key_fingerprint', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'received_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'accepted_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'confirmed_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'discarded_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'discarded_by', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'duration', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_latest_column('apifull_call', 'reason', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));

DROP PROCEDURE apifull_ensure_latest_column;
