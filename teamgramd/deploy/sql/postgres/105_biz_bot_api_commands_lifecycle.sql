-- HTTP Bot API scoped command variants and per-bot lifecycle state.
CREATE TABLE IF NOT EXISTS bot_api_commands (
    bot_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_key TEXT NOT NULL,
    language_code TEXT NOT NULL DEFAULT '',
    commands JSONB NOT NULL DEFAULT '[]'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (bot_id, scope_key, language_code),
    CONSTRAINT bot_api_commands_scope_check CHECK (char_length(scope_key) > 0),
    CONSTRAINT bot_api_commands_array_check CHECK (jsonb_typeof(commands) = 'array')
);

CREATE TABLE IF NOT EXISTS bot_api_lifecycle (
    bot_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    state TEXT NOT NULL DEFAULT 'active',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT bot_api_lifecycle_state_check CHECK (state IN ('active', 'logged_out', 'closed'))
);
