-- PostgreSQL 18 owner-scoped collectible emoji status catalog.
-- Rows are written by the collectible-status provider; APIFull only serves
-- statuses belonging to the authenticated owner.
CREATE TABLE IF NOT EXISTS apifull_collectible_emoji_status (
    owner_user_id BIGINT NOT NULL,
    collectible_id BIGINT NOT NULL CHECK (collectible_id > 0),
    document_id BIGINT NOT NULL CHECK (document_id > 0),
    title TEXT NOT NULL DEFAULT '',
    slug TEXT NOT NULL DEFAULT '',
    pattern_document_id BIGINT NOT NULL DEFAULT 0,
    center_color INTEGER NOT NULL DEFAULT 0,
    edge_color INTEGER NOT NULL DEFAULT 0,
    pattern_color INTEGER NOT NULL DEFAULT 0,
    text_color INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, collectible_id),
    UNIQUE (owner_user_id, document_id)
);

CREATE INDEX IF NOT EXISTS apifull_collectible_emoji_status_owner_idx
    ON apifull_collectible_emoji_status (owner_user_id, collectible_id);
