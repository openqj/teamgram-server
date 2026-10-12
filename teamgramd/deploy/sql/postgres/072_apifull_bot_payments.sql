-- PostgreSQL 18 state for bot payment callback queries and delivery intent.
-- A query is owned by one bot/buyer pair and can advance only through the
-- Layer 229 shipping -> precheckout stages. Callback payloads are retained
-- as typed JSON so a payment worker can resume after a process restart.

CREATE TABLE IF NOT EXISTS apifull_bot_payment_query (
    query_id BIGINT PRIMARY KEY CHECK (query_id > 0),
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    buyer_user_id BIGINT NOT NULL CHECK (buyer_user_id > 0),
    stage TEXT NOT NULL CHECK (stage IN (
        'shipping_pending', 'shipping_answered',
        'precheckout_pending', 'precheckout_answered'
    )),
    payload BYTEA NOT NULL DEFAULT ''::bytea,
    currency VARCHAR(16) NOT NULL DEFAULT '',
    amount BIGINT NOT NULL DEFAULT 0 CHECK (amount >= 0),
    shipping_error TEXT NOT NULL DEFAULT '',
    shipping_options JSONB NOT NULL DEFAULT '[]'::jsonb,
    precheckout_success BOOLEAN,
    precheckout_error TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT apifull_bot_payment_query_shipping_options_array
        CHECK (jsonb_typeof(shipping_options) = 'array'),
    CONSTRAINT apifull_bot_payment_query_precheckout_result
        CHECK (stage <> 'precheckout_answered' OR precheckout_success IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_apifull_bot_payment_query_owner_stage
    ON apifull_bot_payment_query (bot_user_id, stage, expires_at);

CREATE INDEX IF NOT EXISTS idx_apifull_bot_payment_query_expiry
    ON apifull_bot_payment_query (expires_at);

CREATE TABLE IF NOT EXISTS apifull_bot_payment_outbox (
    id BIGSERIAL PRIMARY KEY,
    query_id BIGINT NOT NULL REFERENCES apifull_bot_payment_query(query_id) ON DELETE CASCADE,
    bot_user_id BIGINT NOT NULL CHECK (bot_user_id > 0),
    buyer_user_id BIGINT NOT NULL CHECK (buyer_user_id > 0),
    event_type TEXT NOT NULL CHECK (event_type IN (
        'shipping_query', 'precheckout_query',
        'shipping_answered', 'precheckout_answered'
    )),
    payload JSONB NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending'
        CHECK (state IN ('pending', 'inflight', 'complete')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_until TIMESTAMPTZ,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    CONSTRAINT apifull_bot_payment_outbox_payload_object
        CHECK (jsonb_typeof(payload) = 'object'),
    CONSTRAINT apifull_bot_payment_outbox_identity_match
        CHECK (bot_user_id > 0 AND buyer_user_id > 0),
    CONSTRAINT apifull_bot_payment_outbox_completed_state
        CHECK (completed_at IS NULL OR state = 'complete')
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_apifull_bot_payment_outbox_query_event
    ON apifull_bot_payment_outbox (query_id, event_type);

CREATE INDEX IF NOT EXISTS idx_apifull_bot_payment_outbox_due
    ON apifull_bot_payment_outbox (state, next_attempt_at, lease_until, id);
