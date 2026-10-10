-- PostgreSQL 18 temporary payment-password state.
-- The clear token is never persisted; only its SHA-256 digest is stored.
CREATE TABLE IF NOT EXISTS apifull_tmp_password (
    user_id BIGINT PRIMARY KEY,
    token_digest BYTEA NOT NULL,
    valid_until BIGINT NOT NULL,
    created_at BIGINT NOT NULL
);
