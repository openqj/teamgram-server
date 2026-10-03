-- Durable WebAuthn ceremony state and registered credentials.
CREATE TABLE IF NOT EXISTS apifull_passkey_session (
  challenge VARCHAR(255) NOT NULL PRIMARY KEY,
  user_id BIGINT NOT NULL DEFAULT 0,
  kind VARCHAR(16) NOT NULL,
  data MEDIUMBLOB NOT NULL,
  expires_at BIGINT NOT NULL,
  used TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL,
  KEY idx_apifull_passkey_session_expiry (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS apifull_passkey_credential (
  credential_id VARBINARY(255) NOT NULL PRIMARY KEY,
  user_id BIGINT NOT NULL,
  name VARCHAR(255) NOT NULL DEFAULT '',
  date_created BIGINT NOT NULL,
  last_usage_date BIGINT NOT NULL DEFAULT 0,
  sign_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
  credential MEDIUMBLOB NOT NULL,
  deleted TINYINT NOT NULL DEFAULT 0,
  KEY idx_apifull_passkey_credential_user (user_id, deleted)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
