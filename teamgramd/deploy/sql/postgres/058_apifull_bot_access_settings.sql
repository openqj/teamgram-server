-- PostgreSQL 18 bot access policy state.
-- The list is JSONB so its ordering is preserved for deterministic TL output.
CREATE TABLE IF NOT EXISTS apifull_bot_access_settings (
    bot_user_id BIGINT PRIMARY KEY,
    restricted BOOLEAN NOT NULL DEFAULT FALSE,
    add_users JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_bot_access_settings_users_array CHECK (jsonb_typeof(add_users) = 'array')
);
