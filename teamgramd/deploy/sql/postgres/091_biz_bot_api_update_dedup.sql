-- Durable deduplication keys for Bot API payment query updates.
-- Queue rows can be deleted after getUpdates/webhook delivery, so query keys
-- live separately and remain authoritative across outbox retries.
CREATE TABLE IF NOT EXISTS bot_api_update_dedup (
    bot_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    dedupe_key TEXT NOT NULL CHECK (length(dedupe_key) > 0),
    first_update_id BIGINT NOT NULL CHECK (first_update_id > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (bot_id, dedupe_key)
);

CREATE INDEX IF NOT EXISTS bot_api_update_dedup_bot_created_idx
    ON bot_api_update_dedup (bot_id, created_at);
