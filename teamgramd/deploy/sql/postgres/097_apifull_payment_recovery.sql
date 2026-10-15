-- PostgreSQL 18 durable payment recovery state. Webhook delivery and provider
-- reconciliation are independent of the HTTP request that first settled a
-- payment, so a process restart can resume them from these rows.

CREATE TABLE IF NOT EXISTS apifull_payment_provider_webhook (
    event_id VARCHAR(191) PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL CHECK (event_type IN ('payment.refunded', 'payment.revoked')),
    provider VARCHAR(32) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    request_key VARCHAR(191) NOT NULL DEFAULT '',
    transaction_id VARCHAR(191) NOT NULL,
    product VARCHAR(64) NOT NULL CHECK (product IN ('premium_subscription', 'stars_topup', 'bot_invoice')),
    months INTEGER NOT NULL DEFAULT 0 CHECK (months >= 0),
    stars BIGINT NOT NULL DEFAULT 0 CHECK (stars >= 0),
    reason VARCHAR(255) NOT NULL DEFAULT '',
    occurred_at BIGINT NOT NULL,
    payload_hash CHAR(64) NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    state VARCHAR(16) NOT NULL CHECK (state IN ('pending', 'inflight', 'complete', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_until BIGINT NOT NULL DEFAULT 0,
    claim_token VARCHAR(64) NOT NULL DEFAULT '',
    next_attempt_at BIGINT NOT NULL,
    last_error VARCHAR(255) NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_apifull_payment_provider_webhook_due
    ON apifull_payment_provider_webhook (state, next_attempt_at, created_at);

CREATE TABLE IF NOT EXISTS apifull_payment_provider_reversal (
    provider VARCHAR(32) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    transaction_id VARCHAR(191) NOT NULL,
    event_id VARCHAR(191) NOT NULL,
    product VARCHAR(64) NOT NULL CHECK (product IN ('premium_subscription', 'stars_topup', 'bot_invoice')),
    stars BIGINT NOT NULL DEFAULT 0 CHECK (stars >= 0),
    state VARCHAR(16) NOT NULL CHECK (state IN ('applied')),
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (provider, transaction_id)
);

CREATE TABLE IF NOT EXISTS apifull_payment_provider_reconciliation (
    provider VARCHAR(32) NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    transaction_id VARCHAR(191) NOT NULL,
    request_key VARCHAR(191) NOT NULL DEFAULT '',
    product VARCHAR(64) NOT NULL CHECK (product IN ('premium_subscription', 'stars_topup', 'bot_invoice')),
    months INTEGER NOT NULL DEFAULT 0 CHECK (months >= 0),
    stars BIGINT NOT NULL DEFAULT 0 CHECK (stars >= 0),
    provider_status VARCHAR(32) NOT NULL DEFAULT '',
    state VARCHAR(16) NOT NULL CHECK (state IN ('pending', 'inflight', 'complete', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    lease_until BIGINT NOT NULL DEFAULT 0,
    claim_token VARCHAR(64) NOT NULL DEFAULT '',
    next_check_at BIGINT NOT NULL,
    last_checked_at BIGINT NOT NULL DEFAULT 0,
    last_error VARCHAR(255) NOT NULL DEFAULT '',
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (provider, user_id, transaction_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_payment_provider_reconciliation_due
    ON apifull_payment_provider_reconciliation (state, next_check_at, created_at);
