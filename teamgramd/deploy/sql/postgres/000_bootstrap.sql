-- PostgreSQL 18 baseline for a clean deployment.
-- The application tables are added by later service-owned migrations.

CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    checksum text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now()
);
