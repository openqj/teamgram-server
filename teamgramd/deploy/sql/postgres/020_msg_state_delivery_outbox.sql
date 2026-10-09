-- Message state mutations commit their update log and durable publications.
CREATE TABLE IF NOT EXISTS msg_state_delivery_outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id BIGINT NOT NULL,
    method TEXT NOT NULL,
    payload JSONB NOT NULL,
    state SMALLINT NOT NULL DEFAULT 0 CHECK (state IN (0,1)),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_until TIMESTAMPTZ,
    claim_token UUID,
    last_error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS msg_state_delivery_due_idx
    ON msg_state_delivery_outbox (available_at,id) WHERE state=0;
CREATE INDEX IF NOT EXISTS msg_state_delivery_user_idx
    ON msg_state_delivery_outbox (user_id,id) WHERE state=0;

CREATE TABLE IF NOT EXISTS msg_inbox_consumer_receipts (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    partition INTEGER NOT NULL,
    message_offset BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    operation TEXT NOT NULL,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consumer_group,topic,partition,message_offset,user_id,operation)
);
