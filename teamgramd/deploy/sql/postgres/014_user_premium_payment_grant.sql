-- PostgreSQL 18 additive schema for idempotent premium payment grants.
-- Kept separate from the original user migration so applied checksums remain
-- immutable on installations that already recorded 003_biz_user.

CREATE TABLE IF NOT EXISTS user_premium_payment_grant (
    transaction_key BYTEA PRIMARY KEY,
    provider TEXT NOT NULL,
    transaction_id TEXT NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    months INTEGER NOT NULL,
    created_at BIGINT NOT NULL
);

CREATE INDEX IF NOT EXISTS user_premium_payment_grant_user_idx
    ON user_premium_payment_grant (user_id, created_at);
