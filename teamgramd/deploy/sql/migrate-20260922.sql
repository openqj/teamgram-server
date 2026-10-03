CREATE TABLE IF NOT EXISTS `dialog_filter_tags` (
  `user_id` bigint NOT NULL,
  `enabled` tinyint NOT NULL DEFAULT 0,
  PRIMARY KEY (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
