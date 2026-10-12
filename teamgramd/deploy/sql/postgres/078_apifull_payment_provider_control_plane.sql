-- PostgreSQL 18 state owned by the signed payment/entitlement provider.
-- Provider retries are keyed by operation, caller and request key. The
-- payload hash prevents an idempotency key from being reused for another
-- purchase or gift mutation.
CREATE TABLE IF NOT EXISTS apifull_payment_provider_operation (
    operation TEXT NOT NULL CHECK (char_length(operation) BETWEEN 1 AND 128),
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    request_key TEXT NOT NULL CHECK (char_length(request_key) BETWEEN 1 AND 512),
    payload_hash CHAR(64) NOT NULL CHECK (payload_hash ~ '^[0-9a-f]{64}$'),
    response JSONB NOT NULL CHECK (jsonb_typeof(response) = 'object'),
    signature_header TEXT NOT NULL CHECK (signature_header IN (
        'X-Teamgram-Payment-Signature',
        'X-Teamgram-Payment-Provider-Signature',
        'X-Teamgram-Business-Provider-Signature'
    )),
    transaction_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (operation, user_id, request_key)
);

CREATE INDEX IF NOT EXISTS idx_apifull_payment_provider_operation_transaction
    ON apifull_payment_provider_operation (operation, transaction_id)
    WHERE transaction_id <> '';

CREATE TABLE IF NOT EXISTS apifull_payment_provider_receipt (
    operation TEXT NOT NULL,
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    request_key TEXT NOT NULL,
    transaction_id TEXT NOT NULL CHECK (char_length(transaction_id) BETWEEN 1 AND 512),
    response JSONB NOT NULL CHECK (jsonb_typeof(response) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (operation, transaction_id),
    FOREIGN KEY (operation, user_id, request_key)
        REFERENCES apifull_payment_provider_operation(operation, user_id, request_key)
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS apifull_payment_provider_entitlement (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    product TEXT NOT NULL CHECK (char_length(product) BETWEEN 1 AND 128),
    transaction_id TEXT NOT NULL CHECK (char_length(transaction_id) BETWEEN 1 AND 512),
    months BIGINT NOT NULL DEFAULT 0 CHECK (months >= 0),
    stars BIGINT NOT NULL DEFAULT 0 CHECK (stars >= 0),
    state TEXT NOT NULL CHECK (state IN ('granted', 'revoked')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, product, transaction_id),
    CHECK (months > 0 OR stars > 0)
);

CREATE INDEX IF NOT EXISTS idx_apifull_payment_provider_entitlement_user
    ON apifull_payment_provider_entitlement (user_id, product, updated_at DESC);
