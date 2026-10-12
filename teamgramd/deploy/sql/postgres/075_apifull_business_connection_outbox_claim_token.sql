-- Bind business connection outbox completion and retry to the worker that owns
-- the current lease. A stale worker must not settle a lease reclaimed by a
-- newer worker after the original lease expires.

ALTER TABLE apifull_business_connection_outbox
    ADD COLUMN IF NOT EXISTS claim_token UUID;

CREATE INDEX IF NOT EXISTS idx_apifull_business_connection_outbox_claim_token
    ON apifull_business_connection_outbox (id, claim_token)
    WHERE state = 'inflight';
