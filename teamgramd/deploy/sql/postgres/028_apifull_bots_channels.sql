-- PostgreSQL 18 state for bot menu buttons and channel sticker bindings.
-- Both rows are scoped to their owning peer and are safe to replay.

CREATE TABLE IF NOT EXISTS apifull_bot_menu_button (
    owner_user_id BIGINT NOT NULL,
    bot_user_id BIGINT NOT NULL,
    button JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, bot_user_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_bot_menu_button_bot
    ON apifull_bot_menu_button (bot_user_id, owner_user_id);

CREATE TABLE IF NOT EXISTS apifull_channel_sticker_set (
    channel_id BIGINT PRIMARY KEY,
    sticker_set_id BIGINT NOT NULL DEFAULT 0,
    updated_by_user_id BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
