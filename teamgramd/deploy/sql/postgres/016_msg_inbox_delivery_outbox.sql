-- Authoritative message transactions retain each recipient until the inbox
-- consumer confirms persistence and update publication. Kafka publish alone
-- does not complete a delivery.
CREATE TABLE IF NOT EXISTS msg_inbox_delivery_outbox (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    sender_user_id BIGINT NOT NULL,
    sender_message_id INTEGER NOT NULL,
    dialog_message_id BIGINT NOT NULL,
    peer_type INTEGER NOT NULL,
    peer_id BIGINT NOT NULL,
    recipient_user_id BIGINT NOT NULL,
    payload JSONB NOT NULL,
    state SMALLINT NOT NULL DEFAULT 0 CHECK (state IN (0, 1)),
    attempts INTEGER NOT NULL DEFAULT 0,
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    lease_until TIMESTAMPTZ,
    claim_token UUID,
    last_error TEXT NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (sender_user_id, dialog_message_id, recipient_user_id)
);
CREATE INDEX IF NOT EXISTS msg_inbox_delivery_due_idx
    ON msg_inbox_delivery_outbox (available_at, id) WHERE state = 0;
CREATE INDEX IF NOT EXISTS msg_inbox_delivery_recipient_idx
    ON msg_inbox_delivery_outbox (recipient_user_id, id) WHERE state = 0;
