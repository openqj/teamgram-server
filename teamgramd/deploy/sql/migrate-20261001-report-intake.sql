-- Durable moderation/report intake used by Layer 229 report methods.
-- The APIFull runtime migration is idempotent; this standalone migration is
-- provided for deployments that apply schema changes before restarting BFF.
CREATE TABLE IF NOT EXISTS apifull_report (
  id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
  actor_user_id BIGINT NOT NULL,
  kind VARCHAR(64) NOT NULL,
  target_type VARCHAR(32) NOT NULL,
  target_id BIGINT NOT NULL DEFAULT 0,
  dedupe_key CHAR(64) NOT NULL,
  payload MEDIUMBLOB NOT NULL,
  state VARCHAR(16) NOT NULL DEFAULT 'pending',
  created_at INT NOT NULL,
  updated_at INT NOT NULL,
  UNIQUE KEY uniq_apifull_report_dedupe (dedupe_key),
  KEY idx_apifull_report_actor (actor_user_id, id),
  KEY idx_apifull_report_state (state, id),
  KEY idx_apifull_report_target (target_type, target_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
