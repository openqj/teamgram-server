-- Counter rows participate in the authoritative mutation's transaction.
-- A sequence object would retain increments after rollback and create pts gaps.
CREATE TABLE IF NOT EXISTS idgen_counters (
    key TEXT PRIMARY KEY CHECK (length(key) > 0),
    value BIGINT NOT NULL CHECK (value >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
