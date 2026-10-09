-- PostgreSQL 18 durable SMS jobs opt-in state and provider assignments.
-- Assignment ownership is explicit so a client can only fetch or finish its
-- own job; counters are updated transactionally when a job is acknowledged.
CREATE TABLE IF NOT EXISTS apifull_sms_job_member (
    user_id BIGINT PRIMARY KEY,
    joined BOOLEAN NOT NULL DEFAULT FALSE,
    allow_international BOOLEAN NOT NULL DEFAULT FALSE,
    recent_sent INTEGER NOT NULL DEFAULT 0 CHECK (recent_sent >= 0),
    recent_since BIGINT NOT NULL DEFAULT 0,
    total_sent INTEGER NOT NULL DEFAULT 0 CHECK (total_sent >= 0),
    total_since BIGINT NOT NULL DEFAULT 0,
    last_gift_slug TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS apifull_sms_job (
    job_id TEXT PRIMARY KEY,
    user_id BIGINT NOT NULL,
    phone_number TEXT NOT NULL,
    text TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'finished')),
    error_text TEXT,
    assigned_at BIGINT NOT NULL DEFAULT 0,
    finished_at BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_sms_job_user_state
    ON apifull_sms_job (user_id, state, created_at, job_id);
