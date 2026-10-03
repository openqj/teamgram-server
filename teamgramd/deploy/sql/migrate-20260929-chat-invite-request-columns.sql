-- Add join-request state used by channel invite importers.
-- Apply this migration to production before enabling APIFull channel invites.

SET @has_chat_invite_requested := (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'chat_invite_participants'
    AND column_name = 'requested'
);
SET @add_chat_invite_requested := IF(
  @has_chat_invite_requested = 0,
  'ALTER TABLE `chat_invite_participants` ADD COLUMN `requested` TINYINT(1) NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE add_chat_invite_requested_stmt FROM @add_chat_invite_requested;
EXECUTE add_chat_invite_requested_stmt;
DEALLOCATE PREPARE add_chat_invite_requested_stmt;

SET @has_chat_invite_approved_by := (
  SELECT COUNT(*)
  FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'chat_invite_participants'
    AND column_name = 'approved_by'
);
SET @add_chat_invite_approved_by := IF(
  @has_chat_invite_approved_by = 0,
  'ALTER TABLE `chat_invite_participants` ADD COLUMN `approved_by` BIGINT NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE add_chat_invite_approved_by_stmt FROM @add_chat_invite_approved_by;
EXECUTE add_chat_invite_approved_by_stmt;
DEALLOCATE PREPARE add_chat_invite_approved_by_stmt;
