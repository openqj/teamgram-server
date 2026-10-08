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
