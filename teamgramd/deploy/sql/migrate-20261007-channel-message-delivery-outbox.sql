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
