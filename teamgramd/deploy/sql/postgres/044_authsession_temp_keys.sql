-- Persist temporary auth-key lifetimes for PostgreSQL authsession.
-- The base schema contains this column for fresh installs; this idempotent
-- migration also makes already-provisioned development databases compatible.
ALTER TABLE auth_key_infos
    ADD COLUMN IF NOT EXISTS expires_at BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS auth_key_infos_temp_expiry_idx
    ON auth_key_infos (expires_at)
    WHERE deleted = FALSE AND expires_at > 0;
