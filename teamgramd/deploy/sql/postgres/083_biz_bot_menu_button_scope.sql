-- Telegram Bot API menu-button type and per-chat scope.
ALTER TABLE bots
    ADD COLUMN IF NOT EXISTS menu_button_type TEXT NOT NULL DEFAULT 'default';

CREATE TABLE IF NOT EXISTS bot_api_chat_menu_buttons (
    bot_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    chat_id BIGINT NOT NULL,
    button_type TEXT NOT NULL,
    button_text TEXT NOT NULL DEFAULT '',
    button_url TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (bot_id, chat_id),
    CONSTRAINT bot_api_chat_menu_buttons_type_check
        CHECK (button_type IN ('default', 'commands', 'web_app'))
);
