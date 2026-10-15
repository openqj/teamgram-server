-- PostgreSQL 18 additive state for precise payment entitlement recovery.
-- A grant keeps the expiry it contributed so revoking one transaction can
-- recompute the user's remaining paid Premium entitlement transactionally.

ALTER TABLE user_premium_payment_grant
    ADD COLUMN IF NOT EXISTS starts_at BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS expires_at BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS revoked_at BIGINT NOT NULL DEFAULT 0;

-- Grants created before the recovery columns were introduced have no separate
-- start marker. Their creation time is the only deterministic schedule input.
UPDATE user_premium_payment_grant
SET starts_at = created_at
WHERE starts_at = 0;

CREATE INDEX IF NOT EXISTS user_premium_payment_grant_active_idx
    ON user_premium_payment_grant (user_id, revoked_at, expires_at, starts_at);
