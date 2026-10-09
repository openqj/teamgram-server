-- PostgreSQL 18 durable APIFull story state.
--
-- A story owner has one aggregate document because TL story records contain
-- heterogeneous media and nested view/reaction metadata. JSONB keeps the
-- protocol payload lossless while the owner key and update timestamp provide
-- an explicit transactional boundary for reads and mutations.
CREATE TABLE IF NOT EXISTS apifull_story_state (
    user_id BIGINT PRIMARY KEY,
    state JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_story_state_updated
    ON apifull_story_state (updated_at, user_id);
