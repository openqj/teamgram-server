-- Durable delivery terminal state. A dead-lettered intent keeps its payload and
-- failure metadata for diagnosis and an explicit replay operation; it is never
-- removed merely to make backlog gauges look healthy.
ALTER TABLE msg_inbox_delivery_outbox
    ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS terminal_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE msg_state_delivery_outbox
    ADD COLUMN IF NOT EXISTS max_attempts INTEGER NOT NULL DEFAULT 10,
    ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS terminal_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT NOT NULL DEFAULT '';

UPDATE msg_inbox_delivery_outbox
SET max_attempts = 10
WHERE max_attempts IS NULL OR max_attempts < 1;
UPDATE msg_state_delivery_outbox
SET max_attempts = 10
WHERE max_attempts IS NULL OR max_attempts < 1;
UPDATE msg_state_delivery_outbox
SET idempotency_key = 'legacy:' || id::text
WHERE idempotency_key = '';

ALTER TABLE msg_inbox_delivery_outbox
    DROP CONSTRAINT IF EXISTS msg_inbox_delivery_outbox_state_check;
ALTER TABLE msg_inbox_delivery_outbox
    ADD CONSTRAINT msg_inbox_delivery_outbox_state_check CHECK (state IN (0, 1, 2));
ALTER TABLE msg_state_delivery_outbox
    DROP CONSTRAINT IF EXISTS msg_state_delivery_outbox_state_check;
ALTER TABLE msg_state_delivery_outbox
    ADD CONSTRAINT msg_state_delivery_outbox_state_check CHECK (state IN (0, 1, 2));

ALTER TABLE msg_inbox_delivery_outbox
    ADD CONSTRAINT msg_inbox_delivery_outbox_max_attempts_check CHECK (max_attempts > 0);
ALTER TABLE msg_state_delivery_outbox
    ADD CONSTRAINT msg_state_delivery_outbox_max_attempts_check CHECK (max_attempts > 0);

CREATE UNIQUE INDEX IF NOT EXISTS msg_state_delivery_idempotency_idx
    ON msg_state_delivery_outbox (idempotency_key)
    WHERE idempotency_key <> '';
CREATE INDEX IF NOT EXISTS msg_inbox_delivery_dead_letter_idx
    ON msg_inbox_delivery_outbox (dead_lettered_at, id)
    WHERE state = 2;
CREATE INDEX IF NOT EXISTS msg_state_delivery_dead_letter_idx
    ON msg_state_delivery_outbox (dead_lettered_at, id)
    WHERE state = 2;
