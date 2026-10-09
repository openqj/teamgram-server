-- PostgreSQL 18 state for Mini App/WebView permissions and requests.
-- Requests are scoped to the authenticated user and bot; payload is kept as
-- JSONB so deployments can add fields without changing the wire contract.

CREATE TABLE IF NOT EXISTS apifull_mini_bot_permission (
    user_id BIGINT NOT NULL,
    bot_id BIGINT NOT NULL,
    can_send BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, bot_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_mini_bot_permission_bot
    ON apifull_mini_bot_permission (bot_id, user_id);

CREATE TABLE IF NOT EXISTS apifull_webview_request (
    request_id VARCHAR(128) PRIMARY KEY,
    user_id BIGINT NOT NULL,
    bot_id BIGINT NOT NULL,
    kind VARCHAR(64) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_webview_request_owner
    ON apifull_webview_request (user_id, bot_id, kind, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_apifull_webview_request_expiry
    ON apifull_webview_request (expires_at);
