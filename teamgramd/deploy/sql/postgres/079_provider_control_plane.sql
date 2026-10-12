-- PostgreSQL 18 state for the local, signed business and CDN control-plane
-- fixtures.  The fixtures are deployable services, not application fallbacks:
-- they only answer requests for rows explicitly provisioned here.

CREATE TABLE IF NOT EXISTS teamgram_business_live_session (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    random_id BIGINT NOT NULL CHECK (random_id > 0),
    request_fingerprint BYTEA NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    updates JSONB NOT NULL CHECK (jsonb_typeof(updates) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, random_id)
);

CREATE TABLE IF NOT EXISTS teamgram_business_collectible (
    lookup_key TEXT PRIMARY KEY CHECK (char_length(lookup_key) > 0),
    purchase_date INTEGER NOT NULL CHECK (purchase_date > 0),
    currency TEXT NOT NULL DEFAULT '',
    amount BIGINT NOT NULL DEFAULT 0 CHECK (amount >= 0),
    crypto_currency TEXT NOT NULL DEFAULT '',
    crypto_amount BIGINT NOT NULL DEFAULT 0 CHECK (crypto_amount >= 0),
    url TEXT NOT NULL CHECK (char_length(url) > 0),
    active BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (currency <> '' OR crypto_currency <> '')
);

CREATE TABLE IF NOT EXISTS teamgram_business_callback_answer (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    peer_key TEXT NOT NULL CHECK (char_length(peer_key) > 0),
    message_id BIGINT NOT NULL CHECK (message_id > 0),
    data_digest BYTEA NOT NULL CHECK (octet_length(data_digest) = 32),
    answer JSONB NOT NULL CHECK (jsonb_typeof(answer) = 'object'),
    expires_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, peer_key, message_id, data_digest)
);

CREATE INDEX IF NOT EXISTS idx_teamgram_business_callback_expiry
    ON teamgram_business_callback_answer (expires_at);

CREATE TABLE IF NOT EXISTS teamgram_cdn_object (
    file_token BYTEA PRIMARY KEY CHECK (octet_length(file_token) BETWEEN 16 AND 128),
    source_key TEXT NOT NULL UNIQUE CHECK (char_length(source_key) > 0),
    plaintext BYTEA NOT NULL CHECK (octet_length(plaintext) > 0),
    encryption_key BYTEA NOT NULL CHECK (octet_length(encryption_key) = 32),
    encryption_iv BYTEA NOT NULL CHECK (octet_length(encryption_iv) = 16),
    dc_id INTEGER NOT NULL CHECK (dc_id > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_teamgram_cdn_object_expiry
    ON teamgram_cdn_object (expires_at);

CREATE TABLE IF NOT EXISTS teamgram_cdn_config (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    public_keys JSONB NOT NULL CHECK (jsonb_typeof(public_keys) = 'array' AND jsonb_array_length(public_keys) > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
