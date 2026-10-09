-- Durable acknowledgement cursor for messages.receivedMessages.
CREATE TABLE IF NOT EXISTS bff_messages_received_message (
    user_id BIGINT PRIMARY KEY,
    max_id INTEGER NOT NULL DEFAULT 0 CHECK (max_id >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
