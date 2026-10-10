-- Durable Layer 229 URL authorization state. Rows are scoped by the caller;
-- a URL or match code from one account can never mutate another account.
CREATE TABLE IF NOT EXISTS apifull_url_auth (
    owner_user_id BIGINT NOT NULL,
    url_hash BIGINT NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL CHECK (status IN ('requested', 'accepted')),
    match_code TEXT NOT NULL DEFAULT '',
    bot_id BIGINT NOT NULL DEFAULT 0,
    msg_id INTEGER NOT NULL DEFAULT 0,
    button_id INTEGER NOT NULL DEFAULT 0,
    in_app_origin TEXT NOT NULL DEFAULT '',
    date_created INTEGER NOT NULL DEFAULT 0,
    date_active INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (owner_user_id, url_hash)
);

CREATE INDEX IF NOT EXISTS idx_apifull_url_auth_match
    ON apifull_url_auth (owner_user_id, match_code, status);

CREATE INDEX IF NOT EXISTS idx_apifull_url_auth_accepted
    ON apifull_url_auth (owner_user_id, status, date_active, url_hash);
