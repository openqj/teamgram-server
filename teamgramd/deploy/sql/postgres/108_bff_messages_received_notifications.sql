-- Durable notification rows used by messages.receivedMessages.
-- The sync service writes one row before it fans a notification update out to
-- sessions; the messages BFF deletes acknowledged rows in the same transaction.
CREATE TABLE IF NOT EXISTS bff_messages_received_notification (
    user_id BIGINT NOT NULL,
    message_id INTEGER NOT NULL CHECK (message_id > 0),
    flags INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, message_id, flags)
);

CREATE INDEX IF NOT EXISTS bff_messages_received_notification_ack_idx
    ON bff_messages_received_notification (user_id, message_id);
