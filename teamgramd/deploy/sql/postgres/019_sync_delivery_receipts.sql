-- Kafka replay reuses the auth seq/date committed with its PTS and SEQ events.
-- Delivery completion is recorded after fanout, outside the mutation transaction.
CREATE TABLE IF NOT EXISTS sync_delivery_receipts (
    consumer_group TEXT NOT NULL,
    topic TEXT NOT NULL,
    partition_id INTEGER NOT NULL CHECK (partition_id >= 0),
    message_offset BIGINT NOT NULL CHECK (message_offset >= 0),
    user_id BIGINT NOT NULL,
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    delivery_data JSONB,
    delivered BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at TIMESTAMPTZ,
    PRIMARY KEY (consumer_group, topic, partition_id, message_offset, user_id),
    CHECK (NOT delivered OR (delivery_data IS NOT NULL AND delivered_at IS NOT NULL))
);
