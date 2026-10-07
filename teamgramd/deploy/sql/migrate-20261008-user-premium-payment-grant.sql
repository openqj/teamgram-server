CREATE TABLE IF NOT EXISTS `user_premium_payment_grant` (
  `transaction_key` binary(32) NOT NULL,
  `provider` varchar(32) COLLATE utf8mb4_bin NOT NULL,
  `transaction_id` varchar(191) COLLATE utf8mb4_bin NOT NULL,
  `user_id` bigint(20) NOT NULL,
  `months` int(11) NOT NULL,
  `created_at` bigint(20) NOT NULL,
  PRIMARY KEY (`transaction_key`),
  KEY `idx_user_premium_payment_grant_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
