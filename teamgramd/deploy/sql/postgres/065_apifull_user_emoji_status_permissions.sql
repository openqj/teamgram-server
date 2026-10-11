-- PostgreSQL 18 durable permissions for bot-managed user emoji statuses.
-- A permission belongs to the user who grants it and the bot that receives it.
CREATE TABLE IF NOT EXISTS apifull_user_emoji_status_permission (
    owner_user_id BIGINT NOT NULL CHECK (owner_user_id > 0),
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    enabled BOOLEAN NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, bot_user_id)
);

CREATE INDEX IF NOT EXISTS apifull_user_emoji_status_permission_bot_idx
    ON apifull_user_emoji_status_permission (bot_user_id, owner_user_id)
    WHERE enabled;
