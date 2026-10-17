-- Bind locally settled Bot API gifts to their durable payment request.
ALTER TABLE apifull_gift
    ADD COLUMN IF NOT EXISTS payment_request_id BIGINT;

CREATE UNIQUE INDEX IF NOT EXISTS uniq_apifull_gift_payment_request
    ON apifull_gift (payment_request_id)
    WHERE payment_request_id IS NOT NULL;
