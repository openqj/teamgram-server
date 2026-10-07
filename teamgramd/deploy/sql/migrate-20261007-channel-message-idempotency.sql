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
