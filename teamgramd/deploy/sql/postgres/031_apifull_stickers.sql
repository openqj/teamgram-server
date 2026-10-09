-- PostgreSQL 18 authoritative Layer 229 sticker catalog and per-user state.
-- Catalog rows are immutable media metadata owned by the sticker provider;
-- user rows contain only installation/order/recency/favourite state.
CREATE SEQUENCE IF NOT EXISTS apifull_sticker_set_id_seq AS BIGINT;
CREATE SEQUENCE IF NOT EXISTS apifull_sticker_id_seq AS BIGINT;

CREATE TABLE IF NOT EXISTS apifull_sticker_set (
    id BIGINT PRIMARY KEY DEFAULT nextval('apifull_sticker_set_id_seq'),
    access_hash BIGINT NOT NULL,
    owner_user_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    short_name TEXT NOT NULL,
    masks BOOLEAN NOT NULL DEFAULT FALSE,
    emojis BOOLEAN NOT NULL DEFAULT FALSE,
    text_color BOOLEAN NOT NULL DEFAULT FALSE,
    animated BOOLEAN NOT NULL DEFAULT FALSE,
    videos BOOLEAN NOT NULL DEFAULT FALSE,
    creator BOOLEAN NOT NULL DEFAULT FALSE,
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    thumb_document_id BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_sticker_set_short_name_uq UNIQUE (short_name)
);

CREATE INDEX IF NOT EXISTS apifull_sticker_set_owner_idx
    ON apifull_sticker_set (owner_user_id, id);
CREATE INDEX IF NOT EXISTS apifull_sticker_set_search_idx
    ON apifull_sticker_set USING GIN (to_tsvector('simple', title || ' ' || short_name));
CREATE INDEX IF NOT EXISTS apifull_sticker_set_featured_idx
    ON apifull_sticker_set (featured, id);

CREATE TABLE IF NOT EXISTS apifull_sticker (
    id BIGINT PRIMARY KEY DEFAULT nextval('apifull_sticker_id_seq'),
    set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
    access_hash BIGINT NOT NULL,
    position INTEGER NOT NULL DEFAULT 0,
    alt TEXT NOT NULL DEFAULT '',
    keywords TEXT NOT NULL DEFAULT '',
    mask_coords JSONB,
    mime_type TEXT NOT NULL DEFAULT 'image/webp',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    dc_id INTEGER NOT NULL DEFAULT 2,
    file_reference BYTEA NOT NULL DEFAULT ''::bytea,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_sticker_set_position_uq UNIQUE (set_id, position)
);

CREATE INDEX IF NOT EXISTS apifull_sticker_set_idx
    ON apifull_sticker (set_id, position, id);
CREATE INDEX IF NOT EXISTS apifull_sticker_alt_idx
    ON apifull_sticker USING GIN (to_tsvector('simple', alt || ' ' || keywords));

CREATE TABLE IF NOT EXISTS apifull_sticker_user_set (
    user_id BIGINT NOT NULL,
    set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
    installed BOOLEAN NOT NULL DEFAULT TRUE,
    archived BOOLEAN NOT NULL DEFAULT FALSE,
    unread BOOLEAN NOT NULL DEFAULT FALSE,
    order_index INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, set_id)
);

CREATE INDEX IF NOT EXISTS apifull_sticker_user_set_order_idx
    ON apifull_sticker_user_set (user_id, installed, archived, order_index, set_id);

CREATE TABLE IF NOT EXISTS apifull_sticker_user_recent (
    user_id BIGINT NOT NULL,
    sticker_id BIGINT NOT NULL REFERENCES apifull_sticker(id) ON DELETE CASCADE,
    attached BOOLEAN NOT NULL DEFAULT FALSE,
    saved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, sticker_id, attached)
);

CREATE INDEX IF NOT EXISTS apifull_sticker_user_recent_idx
    ON apifull_sticker_user_recent (user_id, attached, saved_at DESC, sticker_id);

CREATE TABLE IF NOT EXISTS apifull_sticker_user_favourite (
    user_id BIGINT NOT NULL,
    sticker_id BIGINT NOT NULL REFERENCES apifull_sticker(id) ON DELETE CASCADE,
    saved_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, sticker_id)
);

CREATE INDEX IF NOT EXISTS apifull_sticker_user_favourite_idx
    ON apifull_sticker_user_favourite (user_id, saved_at DESC, sticker_id);
