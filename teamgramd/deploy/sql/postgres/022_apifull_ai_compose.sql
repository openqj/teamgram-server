-- PostgreSQL 18 durable state for Layer 229 AI compose tones.
CREATE TABLE IF NOT EXISTS apifull_ai_compose_tone (
    user_id BIGINT NOT NULL,
    id BIGINT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL DEFAULT '',
    tone TEXT NOT NULL DEFAULT '',
    slug TEXT NOT NULL DEFAULT '',
    emoji_id BIGINT NOT NULL DEFAULT 0,
    access_hash BIGINT NOT NULL,
    creator BOOLEAN NOT NULL DEFAULT TRUE,
    saved BOOLEAN NOT NULL DEFAULT FALSE,
    display_author BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_ai_compose_tone_saved
    ON apifull_ai_compose_tone (user_id, saved, id);
