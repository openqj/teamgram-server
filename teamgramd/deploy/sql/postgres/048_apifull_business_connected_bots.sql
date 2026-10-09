-- PostgreSQL 18 state for account connected-bot settings and peer controls.
-- The owner and bot/peer keys make updates idempotent and prevent one user's
-- settings from being visible to another user.
CREATE TABLE IF NOT EXISTS apifull_connected_bot (
    owner_user_id BIGINT NOT NULL,
    bot_user_id BIGINT NOT NULL,
    can_reply BOOLEAN NOT NULL DEFAULT FALSE,
    rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipients_bot JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipients JSONB,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, bot_user_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_connected_bot_bot
    ON apifull_connected_bot (bot_user_id, owner_user_id);

CREATE TABLE IF NOT EXISTS apifull_connected_bot_peer (
    owner_user_id BIGINT NOT NULL,
    peer_type SMALLINT NOT NULL CHECK (peer_type IN (0, 1, 2)),
    peer_id BIGINT NOT NULL,
    paused BOOLEAN NOT NULL DEFAULT FALSE,
    disabled BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, peer_type, peer_id)
);
