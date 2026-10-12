-- PostgreSQL 18 authority for Layer 229 Star referral programs and links.
-- Revenue and participant counters are reserved for the settlement worker;
-- the API never invents those values.
CREATE TABLE IF NOT EXISTS apifull_star_ref_program (
    bot_user_id BIGINT PRIMARY KEY CHECK (bot_user_id > 0),
    commission_permille INTEGER NOT NULL CHECK (commission_permille BETWEEN 0 AND 1000),
    duration_months INTEGER CHECK (duration_months IS NULL OR duration_months BETWEEN 1 AND 120),
    end_date INTEGER CHECK (end_date IS NULL OR end_date >= 0),
    daily_revenue_amount BIGINT CHECK (daily_revenue_amount IS NULL OR daily_revenue_amount >= 0),
    daily_revenue_nanos INTEGER CHECK (daily_revenue_nanos IS NULL OR daily_revenue_nanos BETWEEN 0 AND 999999999),
    active BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_star_ref_program_active
    ON apifull_star_ref_program (active, updated_at DESC, bot_user_id);

CREATE TABLE IF NOT EXISTS apifull_star_ref_connection (
    owner_user_id BIGINT NOT NULL CHECK (owner_user_id > 0),
    peer_type INTEGER NOT NULL CHECK (peer_type > 0),
    peer_id BIGINT NOT NULL CHECK (peer_id > 0),
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    referral_url TEXT NOT NULL CHECK (char_length(referral_url) BETWEEN 1 AND 2048),
    connected_at INTEGER NOT NULL CHECK (connected_at > 0),
    participants BIGINT NOT NULL DEFAULT 0 CHECK (participants >= 0),
    revenue BIGINT NOT NULL DEFAULT 0 CHECK (revenue >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, peer_type, peer_id, bot_user_id),
    UNIQUE (owner_user_id, peer_type, peer_id, referral_url)
);

CREATE INDEX IF NOT EXISTS idx_apifull_star_ref_connection_owner
    ON apifull_star_ref_connection (owner_user_id, peer_type, peer_id, revoked, connected_at DESC, referral_url);
