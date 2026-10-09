-- PostgreSQL 18 durable payment contact information.
-- Credential bytes are intentionally not stored; the boolean records only
-- whether a trusted provider has a reusable credential on file.
CREATE TABLE IF NOT EXISTS apifull_payment_saved_info (
    user_id BIGINT PRIMARY KEY,
    name TEXT NOT NULL DEFAULT '',
    phone TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL DEFAULT '',
    credentials_saved BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at BIGINT NOT NULL
);
