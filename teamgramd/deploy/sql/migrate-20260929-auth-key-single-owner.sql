-- Keep the most recently active owner before enforcing one live owner per auth key.
UPDATE auth_users AS auth_user
JOIN (
  SELECT id
  FROM (
    SELECT
      id,
      ROW_NUMBER() OVER (
        PARTITION BY auth_key_id
        ORDER BY date_active DESC, id DESC
      ) AS owner_rank
    FROM auth_users
    WHERE deleted = 0
  ) AS ranked_auth_users
  WHERE owner_rank > 1
) AS duplicate_owner ON duplicate_owner.id = auth_user.id
SET auth_user.deleted = 1,
    auth_user.date_active = 0;

-- The generated column and index are present in newer base dumps.  Keep this
-- migration safe to run against both older dumps and those newer dumps.
SET @active_auth_key_column_exists := (
  SELECT COUNT(*)
  FROM information_schema.COLUMNS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'auth_users'
    AND COLUMN_NAME = 'active_auth_key_id'
);
SET @active_auth_key_column_sql := IF(
  @active_auth_key_column_exists = 0,
  'ALTER TABLE auth_users ADD COLUMN active_auth_key_id BIGINT GENERATED ALWAYS AS (CASE WHEN deleted = 0 THEN auth_key_id ELSE NULL END) STORED',
  'SELECT 1'
);
PREPARE active_auth_key_column_stmt FROM @active_auth_key_column_sql;
EXECUTE active_auth_key_column_stmt;
DEALLOCATE PREPARE active_auth_key_column_stmt;

SET @active_auth_key_index_exists := (
  SELECT COUNT(*)
  FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'auth_users'
    AND INDEX_NAME = 'auth_users_one_active_owner'
);
SET @active_auth_key_index_sql := IF(
  @active_auth_key_index_exists = 0,
  'ALTER TABLE auth_users ADD UNIQUE KEY auth_users_one_active_owner (active_auth_key_id)',
  'SELECT 1'
);
PREPARE active_auth_key_index_stmt FROM @active_auth_key_index_sql;
EXECUTE active_auth_key_index_stmt;
DEALLOCATE PREPARE active_auth_key_index_stmt;
