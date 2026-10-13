CREATE TABLE IF NOT EXISTS bot_api_default_admin_rights (
    bot_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    group_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    broadcast_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
