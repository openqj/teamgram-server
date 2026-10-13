-- PostgreSQL 18 state owned by the recurring Stars subscription provider.
-- Subscription mutations are committed together with their provider operation
-- idempotency row by the payment fixture.
CREATE TABLE IF NOT EXISTS apifull_payment_provider_stars_account (
    user_id BIGINT PRIMARY KEY CHECK (user_id > 0),
    balance BIGINT NOT NULL CHECK (balance >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apifull_payment_provider_stars_subscription (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    subscription_id TEXT NOT NULL CHECK (char_length(subscription_id) BETWEEN 1 AND 512),
    peer JSONB NOT NULL CHECK (jsonb_typeof(peer) = 'object'),
    until_date INTEGER NOT NULL CHECK (until_date > 0),
    pricing_period INTEGER NOT NULL CHECK (pricing_period > 0),
    pricing_amount BIGINT NOT NULL CHECK (pricing_amount >= 0),
    canceled BOOLEAN NOT NULL DEFAULT FALSE,
    can_refulfill BOOLEAN NOT NULL DEFAULT FALSE,
    missing_balance BOOLEAN NOT NULL DEFAULT FALSE,
    bot_canceled BOOLEAN NOT NULL DEFAULT FALSE,
    chat_invite_hash TEXT,
    title TEXT,
    photo JSONB CHECK (photo IS NULL OR jsonb_typeof(photo) = 'object'),
    invoice_slug TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, subscription_id),
    FOREIGN KEY (user_id) REFERENCES apifull_payment_provider_stars_account(user_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_apifull_payment_provider_stars_subscription_until
    ON apifull_payment_provider_stars_subscription (user_id, until_date DESC);
