-- Durable Telegram Bot API update delivery state.
CREATE TABLE IF NOT EXISTS bot_api_update_counters (
    bot_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    next_update_id BIGINT NOT NULL DEFAULT 1 CHECK (next_update_id > 0)
);

CREATE TABLE IF NOT EXISTS bot_api_updates (
    bot_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    update_id BIGINT NOT NULL CHECK (update_id > 0),
    update_data JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (bot_id, update_id)
);

CREATE INDEX IF NOT EXISTS bot_api_updates_bot_created_idx
    ON bot_api_updates (bot_id, update_id);

CREATE TABLE IF NOT EXISTS bot_api_webhooks (
    bot_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    max_connections INTEGER NOT NULL DEFAULT 40 CHECK (max_connections BETWEEN 1 AND 100),
    allowed_updates JSONB NOT NULL DEFAULT '[]'::jsonb,
    secret_token TEXT NOT NULL DEFAULT '',
    last_error_date BIGINT NOT NULL DEFAULT 0,
    last_error_message TEXT NOT NULL DEFAULT '',
    last_synchronization_error_date BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
