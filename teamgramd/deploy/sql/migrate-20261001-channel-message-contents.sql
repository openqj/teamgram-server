-- Durable per-user receipts for channels.readMessageContents.
CREATE TABLE IF NOT EXISTS apifull_channel_message_content_read (
  user_id BIGINT NOT NULL,
  channel_id BIGINT NOT NULL,
  message_id INT NOT NULL,
  read_at INT NOT NULL,
  PRIMARY KEY (user_id, channel_id, message_id),
  KEY idx_apifull_channel_content_read_message (channel_id, message_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
