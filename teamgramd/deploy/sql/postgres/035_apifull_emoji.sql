-- PostgreSQL 18 authoritative emoji keyword, URL, custom-media and group catalog.
-- Catalog rows are provider-owned; handlers only expose rows committed here.
CREATE TABLE IF NOT EXISTS apifull_emoji_language (
    lang_code TEXT PRIMARY KEY,
    version INTEGER NOT NULL DEFAULT 1,
    url TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apifull_emoji_keyword (
    lang_code TEXT NOT NULL REFERENCES apifull_emoji_language(lang_code) ON DELETE CASCADE,
    keyword TEXT NOT NULL,
    emoticons JSONB NOT NULL DEFAULT '[]'::jsonb,
    PRIMARY KEY (lang_code, keyword)
);

CREATE INDEX IF NOT EXISTS apifull_emoji_keyword_lang_idx
    ON apifull_emoji_keyword (lang_code, keyword);

CREATE TABLE IF NOT EXISTS apifull_emoji_document (
    id BIGINT PRIMARY KEY,
    access_hash BIGINT NOT NULL,
    date INTEGER NOT NULL DEFAULT 0,
    mime_type TEXT NOT NULL DEFAULT 'application/x-tgsticker',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    dc_id INTEGER NOT NULL DEFAULT 2,
    file_reference BYTEA NOT NULL DEFAULT ''::bytea,
    alt TEXT NOT NULL DEFAULT '',
    set_id BIGINT NOT NULL DEFAULT 0,
    featured BOOLEAN NOT NULL DEFAULT FALSE,
    profile BOOLEAN NOT NULL DEFAULT FALSE,
    status BOOLEAN NOT NULL DEFAULT FALSE,
    group_photo BOOLEAN NOT NULL DEFAULT FALSE,
    background BOOLEAN NOT NULL DEFAULT FALSE,
    channel_status BOOLEAN NOT NULL DEFAULT FALSE,
    restricted_status BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS apifull_emoji_document_alt_idx
    ON apifull_emoji_document (alt, id);

ALTER TABLE apifull_emoji_document
    ADD COLUMN IF NOT EXISTS group_photo BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE apifull_emoji_document
    ADD COLUMN IF NOT EXISTS background BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE apifull_emoji_document
    ADD COLUMN IF NOT EXISTS channel_status BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE apifull_emoji_document
    ADD COLUMN IF NOT EXISTS restricted_status BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS apifull_emoji_group (
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    icon_emoji_id BIGINT NOT NULL DEFAULT 0,
    emoticons JSONB NOT NULL DEFAULT '[]'::jsonb,
    position INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (kind, title)
);

CREATE INDEX IF NOT EXISTS apifull_emoji_group_order_idx
    ON apifull_emoji_group (kind, position, title);

INSERT INTO apifull_emoji_language (lang_code, version, url)
VALUES ('en', 1, 'https://telegram.org/emoji')
ON CONFLICT (lang_code) DO NOTHING;
