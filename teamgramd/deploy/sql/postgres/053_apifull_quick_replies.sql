-- PostgreSQL 18 durable Business Quick Reply state.
-- One row represents one caller-owned shortcut and its saved message IDs.
CREATE TABLE IF NOT EXISTS apifull_quick_reply (
    user_id BIGINT NOT NULL,
    shortcut_id INTEGER NOT NULL CHECK (shortcut_id > 0),
    shortcut VARCHAR(64) NOT NULL CHECK (char_length(shortcut) > 0),
    message_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
    PRIMARY KEY (user_id, shortcut_id),
    CONSTRAINT uq_apifull_quick_reply_shortcut UNIQUE (user_id, shortcut)
);
CREATE INDEX IF NOT EXISTS idx_apifull_quick_reply_user_position
    ON apifull_quick_reply (user_id, position, shortcut_id);
