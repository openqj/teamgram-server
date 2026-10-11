-- Explicit catalogue projection for unique-gift chat themes.
-- A gift is listed only after an authoritative gift/provider integration marks
-- its durable inventory row as theme-capable; ordinary gifts are not inferred
-- to be themes from their slug or price.
CREATE TABLE IF NOT EXISTS apifull_gift_chat_theme (
    gift_id BIGINT PRIMARY KEY REFERENCES apifull_gift(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
