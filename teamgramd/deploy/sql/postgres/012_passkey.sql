-- PostgreSQL 18 schema for WebAuthn/passkey ceremonies and credentials.
--
-- The passkey service is production-read-only with respect to schema setup;
-- its DAO must find these tables in the deployment migration sequence.

CREATE TABLE IF NOT EXISTS apifull_passkey_session (
    challenge VARCHAR(255) NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL DEFAULT 0,
    kind VARCHAR(16) NOT NULL,
    data BYTEA NOT NULL,
    expires_at BIGINT NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    created_at BIGINT NOT NULL,
    CONSTRAINT apifull_passkey_session_kind_key UNIQUE (challenge, kind)
);

CREATE INDEX IF NOT EXISTS idx_apifull_passkey_session_expiry
    ON apifull_passkey_session (expires_at);

CREATE TABLE IF NOT EXISTS apifull_passkey_credential (
    credential_id BYTEA NOT NULL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL DEFAULT '',
    date_created BIGINT NOT NULL,
    last_usage_date BIGINT NOT NULL DEFAULT 0,
    sign_count BIGINT NOT NULL DEFAULT 0,
    credential BYTEA NOT NULL,
    deleted BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_apifull_passkey_credential_user
    ON apifull_passkey_credential (user_id, deleted);
