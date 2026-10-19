-- Durable signed Stars deltas and idempotent Sub2API usage callbacks.

CREATE TABLE IF NOT EXISTS apifull_stars_sub2api_outbox (
    event_id VARCHAR(191) PRIMARY KEY,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    delta_stars BIGINT NOT NULL CHECK (delta_stars <> 0),
    state VARCHAR(16) NOT NULL CHECK (state IN ('pending', 'inflight', 'complete')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_until TIMESTAMPTZ NOT NULL DEFAULT TO_TIMESTAMP(0),
    claim_token UUID,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_error VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_apifull_stars_sub2api_outbox_due
    ON apifull_stars_sub2api_outbox (state, next_attempt_at, created_at);

CREATE TABLE IF NOT EXISTS apifull_sub2api_stars_residual (
    account_id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    residual_stars NUMERIC NOT NULL CHECK (residual_stars > -1 AND residual_stars < 1),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apifull_sub2api_stars_usage_event (
    event_id VARCHAR(191) PRIMARY KEY,
    account_id VARCHAR(64) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    amount_usd NUMERIC NOT NULL CHECK (amount_usd <> 0),
    currency CHAR(3) NOT NULL CHECK (currency = 'USD'),
    applied_stars_delta BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_sub2api_stars_usage_account
    ON apifull_sub2api_stars_usage_event (account_id, created_at);
