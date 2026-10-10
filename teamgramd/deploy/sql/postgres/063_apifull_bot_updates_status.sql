-- PostgreSQL 18 durable bot update-consumer status.
-- A bot has one current snapshot; retries replace it atomically.
CREATE TABLE IF NOT EXISTS apifull_bot_updates_status (
    bot_user_id BIGINT PRIMARY KEY CHECK (bot_user_id > 0),
    pending_updates_count INTEGER NOT NULL CHECK (pending_updates_count >= 0),
    message TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_bot_updates_status_message_length CHECK (char_length(message) <= 4096)
);
