-- PostgreSQL 18 authority for deployable Bot/Inline/WebView business data.
-- Rows are published by an operations/control-plane job. Request handlers may
-- only read published rows and must validate the resulting Layer 229 type.
CREATE TABLE IF NOT EXISTS apifull_business_control_plane (
    operation TEXT NOT NULL CHECK (char_length(operation) BETWEEN 1 AND 128),
    scope_key TEXT NOT NULL CHECK (char_length(scope_key) BETWEEN 1 AND 2048),
    caller_user_id BIGINT NOT NULL DEFAULT 0 CHECK (caller_user_id >= 0),
    bot_user_id BIGINT CHECK (bot_user_id IS NULL OR bot_user_id > 0),
    query_id BIGINT CHECK (query_id IS NULL OR query_id > 0),
    state TEXT NOT NULL DEFAULT 'published' CHECK (state IN ('published', 'revoked')),
    result JSONB NOT NULL CHECK (jsonb_typeof(result) IN ('object', 'array')),
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (operation, scope_key, caller_user_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_business_control_plane_live
    ON apifull_business_control_plane (operation, scope_key, caller_user_id)
    WHERE state = 'published';

CREATE INDEX IF NOT EXISTS idx_apifull_business_control_plane_expiry
    ON apifull_business_control_plane (expires_at)
    WHERE expires_at IS NOT NULL;
