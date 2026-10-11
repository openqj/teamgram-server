-- PostgreSQL 18 state for connected business-bot identities and their
-- durable update delivery intent.  A connection is owned by the business
-- account and addressed by the bot-visible connection_id.
CREATE TABLE IF NOT EXISTS apifull_business_connection (
    connection_id TEXT PRIMARY KEY CHECK (char_length(connection_id) > 0),
    owner_user_id BIGINT NOT NULL CHECK (owner_user_id > 0),
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    dc_id INTEGER NOT NULL CHECK (dc_id > 0),
    connection_date INTEGER NOT NULL CHECK (connection_date > 0),
    qts INTEGER NOT NULL CHECK (qts > 0),
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    can_reply BOOLEAN NOT NULL DEFAULT FALSE,
    rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (owner_user_id, bot_user_id),
    UNIQUE (bot_user_id, qts),
    CONSTRAINT apifull_business_connection_rights_object
        CHECK (jsonb_typeof(rights) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_apifull_business_connection_bot
    ON apifull_business_connection (bot_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS apifull_business_connection_outbox (
    id BIGSERIAL PRIMARY KEY,
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    connection_id TEXT NOT NULL,
    qts INTEGER NOT NULL CHECK (qts > 0),
    payload JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'inflight', 'complete')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    UNIQUE (bot_user_id, connection_id, qts)
);

CREATE INDEX IF NOT EXISTS idx_apifull_business_connection_outbox_due
    ON apifull_business_connection_outbox (state, next_attempt_at, id);
