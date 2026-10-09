-- PostgreSQL 18 durable boost inventory and target state for Layer 229.
-- A user slot is unique per account and can be reassigned transactionally;
-- target totals are maintained under the same row lock as the assignment.
CREATE TABLE IF NOT EXISTS apifull_boost_target (
    scope TEXT NOT NULL,
    peer_type SMALLINT NOT NULL,
    peer_id BIGINT NOT NULL,
    owner_user_id BIGINT NOT NULL DEFAULT 0,
    boosts INTEGER NOT NULL DEFAULT 0 CHECK (boosts >= 0),
    blocked_boosts INTEGER NOT NULL DEFAULT 0 CHECK (blocked_boosts >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (scope, peer_type, peer_id)
);

CREATE TABLE IF NOT EXISTS apifull_boost_slot (
    user_id BIGINT NOT NULL,
    slot INTEGER NOT NULL CHECK (slot > 0),
    scope TEXT,
    peer_type SMALLINT,
    peer_id BIGINT,
    gift BOOLEAN NOT NULL DEFAULT FALSE,
    giveaway BOOLEAN NOT NULL DEFAULT FALSE,
    unclaimed BOOLEAN NOT NULL DEFAULT FALSE,
    boost_date INTEGER NOT NULL DEFAULT 0,
    expires INTEGER NOT NULL DEFAULT 0,
    cooldown_until INTEGER NOT NULL DEFAULT 0,
    used_gift_slug TEXT NOT NULL DEFAULT '',
    multiplier INTEGER NOT NULL DEFAULT 1,
    stars BIGINT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, slot),
    CHECK ((scope IS NULL AND peer_type IS NULL AND peer_id IS NULL)
        OR (scope IS NOT NULL AND peer_type IS NOT NULL AND peer_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_apifull_boost_target_list
    ON apifull_boost_target (scope, peer_type, peer_id, boosts);

CREATE INDEX IF NOT EXISTS idx_apifull_boost_slot_target
    ON apifull_boost_slot (scope, peer_type, peer_id, user_id, slot);
