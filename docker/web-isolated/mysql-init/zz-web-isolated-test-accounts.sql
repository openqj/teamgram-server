SET @has_requested := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'chat_invite_participants'
    AND column_name = 'requested'
);
SET @add_requested := IF(
  @has_requested = 0,
  'ALTER TABLE chat_invite_participants ADD COLUMN requested TINYINT(1) NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE add_requested_stmt FROM @add_requested;
EXECUTE add_requested_stmt;
DEALLOCATE PREPARE add_requested_stmt;

SET @has_approved_by := (
  SELECT COUNT(*) FROM information_schema.columns
  WHERE table_schema = DATABASE()
    AND table_name = 'chat_invite_participants'
    AND column_name = 'approved_by'
);
SET @add_approved_by := IF(
  @has_approved_by = 0,
  'ALTER TABLE chat_invite_participants ADD COLUMN approved_by BIGINT NOT NULL DEFAULT 0',
  'SELECT 1'
);
PREPARE add_approved_by_stmt FROM @add_approved_by;
EXECUTE add_approved_by_stmt;
DEALLOCATE PREPARE add_approved_by_stmt;

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test One',
  'isolated_test_01',
  '12025550101',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550101');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Two',
  'isolated_test_02',
  '12025550102',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550102');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Three',
  'isolated_test_03',
  '12025550103',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550103');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Four',
  'isolated_test_04',
  '12025550104',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550104');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Five',
  'isolated_test_05',
  '12025550105',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550105');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Six',
  'isolated_test_06',
  '12025550106',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550106');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Seven',
  'isolated_test_07',
  '12025550107',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550107');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Eight',
  'isolated_test_08',
  '12025550108',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550108');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Nine',
  'isolated_test_09',
  '12025550109',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550109');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Ten',
  'isolated_test_10',
  '12025550110',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550110');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Eleven',
  'isolated_test_11',
  '12025550111',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550111');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Twelve',
  'isolated_test_12',
  '12025550112',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550112');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Thirteen',
  'isolated_test_13',
  '12025550113',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550113');

INSERT INTO users
  (user_type, access_hash, secret_key_id, first_name, username, phone, country_code, verified, about, is_bot)
SELECT
  5,
  CAST(CONV(HEX(RANDOM_BYTES(7)), 16, 10) AS SIGNED),
  0,
  'Isolated Test Fourteen',
  'isolated_test_14',
  '12025550114',
  'US',
  1,
  '',
  0
WHERE NOT EXISTS (SELECT 1 FROM users WHERE phone = '12025550114');

INSERT INTO username (username, peer_type, peer_id, editable, active, order2)
SELECT users.username, 2, users.id, TRUE, TRUE, 0
FROM users
WHERE users.phone IN (
  '12025550101',
  '12025550102',
  '12025550103',
  '12025550104',
  '12025550105',
  '12025550106',
  '12025550107',
  '12025550108',
  '12025550109',
  '12025550110',
  '12025550111',
  '12025550112',
  '12025550113',
  '12025550114'
)
  AND users.username <> ''
  AND NOT EXISTS (
    SELECT 1 FROM username
    WHERE username.username = users.username
  );
