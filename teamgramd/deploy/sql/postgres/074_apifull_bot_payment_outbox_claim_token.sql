-- Bind outbox completion and retry to the worker that owns the current lease.
-- A stale worker must not settle a lease that another worker reclaimed.

ALTER TABLE apifull_bot_payment_outbox
    ADD COLUMN IF NOT EXISTS claim_token UUID;

CREATE INDEX IF NOT EXISTS idx_apifull_bot_payment_outbox_claim_token
    ON apifull_bot_payment_outbox (id, claim_token)
    WHERE state = 'inflight';
