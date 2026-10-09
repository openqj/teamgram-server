-- Durable acknowledgement state for messages.reportMessagesDelivery.
CREATE TABLE IF NOT EXISTS apifull_message_delivery_report (
    user_id BIGINT NOT NULL,
    message_id INTEGER NOT NULL,
    received_at BIGINT NOT NULL,
    PRIMARY KEY (user_id, message_id)
);
CREATE INDEX IF NOT EXISTS idx_apifull_message_delivery_report_user_time
    ON apifull_message_delivery_report (user_id, received_at, message_id);
