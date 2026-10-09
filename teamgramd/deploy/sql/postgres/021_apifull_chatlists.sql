-- PostgreSQL 18 Chatlists state and public invite index.
-- State and invite rows are replaced in one transaction by the APIFull domain.
CREATE TABLE IF NOT EXISTS apifull_chatlist_state (
    user_id BIGINT PRIMARY KEY,
    state JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apifull_chatlist_invite (
    slug TEXT PRIMARY KEY,
    owner_user_id BIGINT NOT NULL,
    filter_id INTEGER NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    peers JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS apifull_chatlist_invite_owner_idx
    ON apifull_chatlist_invite (owner_user_id, slug);
