CREATE TABLE IF NOT EXISTS apifull_kv (
  k VARCHAR(191) NOT NULL PRIMARY KEY,
  v MEDIUMTEXT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel (
  id BIGINT NOT NULL PRIMARY KEY,
  access_hash BIGINT NOT NULL,
  migrated_from_chat_id BIGINT NULL,
  creator_user_id BIGINT NOT NULL,
  title VARCHAR(255) NOT NULL,
  about VARCHAR(1024) NOT NULL DEFAULT '',
  broadcast TINYINT NOT NULL DEFAULT 0,
  megagroup TINYINT NOT NULL DEFAULT 0,
  location_lat DOUBLE NULL,
  location_long DOUBLE NULL,
  location_address VARCHAR(255) NOT NULL DEFAULT '',
  photo_id BIGINT NOT NULL DEFAULT 0,
  photo_dc_id INT NOT NULL DEFAULT 0,
  photo_has_video TINYINT NOT NULL DEFAULT 0,
  username VARCHAR(64) NOT NULL DEFAULT '',
  discussion_group_id BIGINT NULL,
  created_at INT NOT NULL,
  KEY idx_apifull_channel_creator (creator_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_member (
  channel_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  invited_by_user_id BIGINT NOT NULL DEFAULT 0,
  joined_at INT NOT NULL,
  admin_rights VARCHAR(2048) NOT NULL DEFAULT '',
  admin_rank VARCHAR(64) NOT NULL DEFAULT '',
  banned_rights VARCHAR(2048) NOT NULL DEFAULT '',
  banned_by_user_id BIGINT NOT NULL DEFAULT 0,
  banned_at INT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, user_id),
  KEY idx_apifull_channel_member_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

DROP PROCEDURE IF EXISTS apifull_ensure_column;
DELIMITER //
CREATE PROCEDURE apifull_ensure_column(
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
    SET @apifull_ddl = CONCAT(
      'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
      '` ADD COLUMN `', REPLACE(p_column_name, '`', '``'), '` ', p_definition
    );
    PREPARE apifull_stmt FROM @apifull_ddl;
    EXECUTE apifull_stmt;
    DEALLOCATE PREPARE apifull_stmt;
  END IF;
END//
DELIMITER ;

DROP PROCEDURE IF EXISTS apifull_ensure_index;
DELIMITER //
CREATE PROCEDURE apifull_ensure_index(
  IN p_table_name VARCHAR(64),
  IN p_index_name VARCHAR(64),
  IN p_columns VARCHAR(255)
)
BEGIN
  DECLARE index_count INT DEFAULT 0;

  SELECT COUNT(*) INTO index_count
  FROM information_schema.statistics
  WHERE table_schema = DATABASE()
    AND table_name = p_table_name
    AND index_name = p_index_name;

  IF index_count = 0 THEN
    SET @apifull_ddl = CONCAT(
      'ALTER TABLE `', REPLACE(p_table_name, '`', '``'),
      '` ADD UNIQUE KEY `', REPLACE(p_index_name, '`', '``'), '` (', p_columns, ')'
    );
    PREPARE apifull_stmt FROM @apifull_ddl;
    EXECUTE apifull_stmt;
    DEALLOCATE PREPARE apifull_stmt;
  END IF;
END//
DELIMITER ;

CALL apifull_ensure_column('apifull_channel', 'migrated_from_chat_id', 'BIGINT NULL');
ALTER TABLE apifull_channel MODIFY COLUMN migrated_from_chat_id BIGINT NULL DEFAULT NULL;
CALL apifull_ensure_column('apifull_channel_member', 'admin_rights', CONCAT('VARCHAR(2048) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_channel_member', 'admin_rank', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_channel_member', 'banned_rights', CONCAT('VARCHAR(2048) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_channel_member', 'banned_by_user_id', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel_member', 'banned_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel', 'location_lat', 'DOUBLE NULL');
CALL apifull_ensure_column('apifull_channel', 'location_long', 'DOUBLE NULL');
CALL apifull_ensure_column('apifull_channel', 'location_address', CONCAT('VARCHAR(255) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_channel', 'photo_id', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel', 'photo_dc_id', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel', 'photo_has_video', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel', 'username', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_channel', 'discussion_group_id', 'BIGINT NULL');
CALL apifull_ensure_index('apifull_channel', 'uniq_apifull_channel_migrated_chat', 'migrated_from_chat_id');

DROP PROCEDURE IF EXISTS apifull_ensure_index;

CREATE TABLE IF NOT EXISTS apifull_channel_message (
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  date INT NOT NULL,
  message TEXT NOT NULL,
  edited TINYINT NOT NULL DEFAULT 0,
  edited_at INT NOT NULL DEFAULT 0,
  reply_to_msg_id INT NOT NULL DEFAULT 0,
  reply_to_top_id INT NOT NULL DEFAULT 0,
  pinned TINYINT NOT NULL DEFAULT 0,
  PRIMARY KEY (channel_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_message_hidden (
  user_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  PRIMARY KEY (user_id, channel_id, message_id),
  KEY idx_apifull_channel_hidden_message (channel_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_message_seq (
  channel_id BIGINT NOT NULL PRIMARY KEY,
  last_message_id INT NOT NULL,
  pts INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_channel_read_state (
  user_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL,
  read_max_id INT NOT NULL DEFAULT 0,
  PRIMARY KEY (user_id, channel_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CALL apifull_ensure_column('apifull_channel_message', 'pinned', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel_message', 'reply_to_msg_id', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_channel_message', 'reply_to_top_id', 'INT NOT NULL DEFAULT 0');

CREATE TABLE IF NOT EXISTS apifull_call (
  id BIGINT NOT NULL PRIMARY KEY,
  access_hash BIGINT NOT NULL,
  admin_id BIGINT NOT NULL,
  participant_id BIGINT NOT NULL,
  state VARCHAR(32) NOT NULL,
  video TINYINT NOT NULL DEFAULT 0,
  ga_hash VARBINARY(256) NULL,
  gb VARBINARY(256) NULL,
  ga VARBINARY(256) NULL,
  protocol MEDIUMTEXT NULL,
  key_fingerprint BIGINT NOT NULL DEFAULT 0,
  received_at INT NOT NULL DEFAULT 0,
  accepted_at INT NOT NULL DEFAULT 0,
  confirmed_at INT NOT NULL DEFAULT 0,
  discarded_at INT NOT NULL DEFAULT 0,
  discarded_by BIGINT NOT NULL DEFAULT 0,
  duration INT NOT NULL DEFAULT 0,
  reason VARCHAR(64) NOT NULL DEFAULT '',
  created_at INT NOT NULL,
  KEY idx_apifull_call_participant (participant_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_call_artifact (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  call_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  kind VARCHAR(32) NOT NULL,
  payload MEDIUMBLOB NOT NULL,
  created_at INT NOT NULL,
  KEY idx_apifull_call_artifact_call (call_id, kind, id),
  KEY idx_apifull_call_artifact_user (user_id, kind, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CALL apifull_ensure_column('apifull_call', 'ga_hash', 'VARBINARY(256) NULL');
CALL apifull_ensure_column('apifull_call', 'gb', 'VARBINARY(256) NULL');
CALL apifull_ensure_column('apifull_call', 'ga', 'VARBINARY(256) NULL');
CALL apifull_ensure_column('apifull_call', 'protocol', 'MEDIUMTEXT NULL');
CALL apifull_ensure_column('apifull_call', 'key_fingerprint', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'received_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'accepted_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'confirmed_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'discarded_at', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'discarded_by', 'BIGINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'duration', 'INT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_call', 'reason', CONCAT('VARCHAR(64) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));

CREATE TABLE IF NOT EXISTS apifull_stars (
  user_id BIGINT NOT NULL PRIMARY KEY,
  balance BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_star_tx (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  amount BIGINT NOT NULL,
  idem VARCHAR(191) NOT NULL,
  UNIQUE KEY uniq_apifull_star_tx (user_id, idem)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_gift (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  from_user BIGINT NOT NULL,
  to_user BIGINT NOT NULL,
  slug VARCHAR(191) NOT NULL,
  stars BIGINT NOT NULL DEFAULT 0,
  saved TINYINT NOT NULL DEFAULT 1,
  KEY idx_apifull_gift_to (to_user)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_username (
  username VARCHAR(64) NOT NULL PRIMARY KEY,
  owner_user_id BIGINT NOT NULL,
  kind VARCHAR(16) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call (
  id BIGINT NOT NULL PRIMARY KEY,
  access_hash BIGINT NOT NULL,
  creator_user_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL DEFAULT 0,
  title VARCHAR(255) NOT NULL DEFAULT '',
  rtmp_stream TINYINT NOT NULL DEFAULT 0,
  conference TINYINT NOT NULL DEFAULT 0,
  schedule_date INT NULL,
  participants MEDIUMTEXT NOT NULL,
  created_at INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_settings (
  call_id BIGINT NOT NULL PRIMARY KEY,
  join_muted TINYINT NOT NULL DEFAULT 0,
  messages_enabled TINYINT NOT NULL DEFAULT 1,
  send_paid_messages_stars BIGINT NULL,
  record_active TINYINT NOT NULL DEFAULT 0,
  record_video TINYINT NOT NULL DEFAULT 0,
  record_title VARCHAR(255) NOT NULL DEFAULT '',
  record_video_portrait TINYINT NOT NULL DEFAULT 0,
  scheduled_started TINYINT NOT NULL DEFAULT 0,
  updated_at INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_invite (
  call_id BIGINT NOT NULL PRIMARY KEY,
  creator_user_id BIGINT NOT NULL,
  token_hash VARBINARY(32) NOT NULL,
  can_self_unmute TINYINT NOT NULL DEFAULT 0,
  revoked_at INT NOT NULL DEFAULT 0,
  created_at INT NOT NULL,
  UNIQUE KEY uniq_apifull_group_call_invite_token (token_hash),
  KEY idx_apifull_group_call_invite_creator (creator_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_conference (
  call_id BIGINT NOT NULL PRIMARY KEY,
  public_key VARBINARY(256) NOT NULL,
  block MEDIUMBLOB NOT NULL,
  params MEDIUMTEXT NOT NULL,
  updated_at INT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_participant (
  call_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  muted TINYINT NOT NULL DEFAULT 0,
  volume INT NULL,
  raise_hand TINYINT NOT NULL DEFAULT 0,
  video_stopped TINYINT NOT NULL DEFAULT 0,
  video_paused TINYINT NOT NULL DEFAULT 0,
  presentation_paused TINYINT NOT NULL DEFAULT 0,
  presentation_active TINYINT NOT NULL DEFAULT 0,
  presentation_params MEDIUMTEXT NOT NULL,
  join_params MEDIUMTEXT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (call_id, user_id),
  KEY idx_apifull_group_call_participant_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_subscription (
  call_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  subscribed TINYINT NOT NULL DEFAULT 0,
  updated_at INT NOT NULL,
  PRIMARY KEY (call_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_send_as (
  call_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  send_as MEDIUMTEXT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (call_id, user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_group_call_message (
  id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  call_id BIGINT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  random_id BIGINT NOT NULL,
  message MEDIUMTEXT NOT NULL,
  send_as MEDIUMTEXT NOT NULL,
  paid_stars BIGINT NULL,
  date INT NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  UNIQUE KEY uniq_apifull_group_call_message_random (call_id, random_id),
  KEY idx_apifull_group_call_message_call (call_id, id),
  KEY idx_apifull_group_call_message_sender (call_id, sender_user_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_secret_chat (
  id INT NOT NULL PRIMARY KEY,
  access_hash BIGINT NOT NULL,
  admin_user_id BIGINT NOT NULL,
  participant_user_id BIGINT NOT NULL,
  state VARCHAR(16) NOT NULL,
  g_a VARBINARY(256) NOT NULL,
  g_b VARBINARY(256) NULL,
  key_fingerprint BIGINT NOT NULL DEFAULT 0,
  created_at INT NOT NULL,
  accepted_at INT NOT NULL DEFAULT 0,
  discarded_at INT NOT NULL DEFAULT 0,
  discarded_by_user_id BIGINT NOT NULL DEFAULT 0,
  history_deleted TINYINT NOT NULL DEFAULT 0,
  KEY idx_apifull_secret_chat_admin (admin_user_id),
  KEY idx_apifull_secret_chat_participant (participant_user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_secret_chat_device_key (
  chat_id INT NOT NULL,
  user_id BIGINT NOT NULL,
  device_id BIGINT NOT NULL,
  epoch INT NOT NULL,
  public_key VARBINARY(256) NOT NULL,
  fingerprint BIGINT NOT NULL DEFAULT 0,
  created_at INT NOT NULL,
  PRIMARY KEY (chat_id, user_id, device_id, epoch),
  KEY idx_apifull_secret_device_key (chat_id, user_id, device_id, epoch)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_secret_user_state (
  user_id BIGINT NOT NULL PRIMARY KEY,
  last_qts INT NOT NULL DEFAULT 0,
  confirmed_qts INT NOT NULL DEFAULT 0
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_secret_message (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  chat_id INT NOT NULL,
  sender_user_id BIGINT NOT NULL,
  recipient_user_id BIGINT NOT NULL,
  random_id BIGINT NOT NULL,
  qts INT NOT NULL,
  date INT NOT NULL,
  encrypted_data MEDIUMBLOB NOT NULL,
  service TINYINT NOT NULL DEFAULT 0,
  file_id BIGINT NULL,
  file_access_hash BIGINT NULL,
  file_size BIGINT NULL,
  file_dc_id INT NULL,
  file_key_fingerprint INT NULL,
  acknowledged_at INT NOT NULL DEFAULT 0,
  read_at INT NOT NULL DEFAULT 0,
  UNIQUE KEY uniq_apifull_secret_sender_random (sender_user_id, random_id),
  UNIQUE KEY uniq_apifull_secret_recipient_qts (recipient_user_id, qts),
  KEY idx_apifull_secret_message_chat (chat_id),
  KEY idx_apifull_secret_message_recipient (recipient_user_id, acknowledged_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CALL apifull_ensure_column('apifull_group_call', 'title', CONCAT('VARCHAR(255) NOT NULL DEFAULT ', CHAR(39), CHAR(39)));
CALL apifull_ensure_column('apifull_group_call', 'rtmp_stream', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_group_call', 'conference', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_group_call', 'schedule_date', 'INT NULL');
CALL apifull_ensure_column('apifull_group_call_participant', 'presentation_active', 'TINYINT NOT NULL DEFAULT 0');
CALL apifull_ensure_column('apifull_group_call_participant', 'join_params', 'MEDIUMTEXT NOT NULL');

DROP PROCEDURE apifull_ensure_column;
