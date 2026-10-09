-- PostgreSQL 18 persistence for bots.setBot{Group,Broadcast}DefaultAdminRights.
-- The two constructor payloads are kept together so each bot has one
-- authoritative defaults record and each update is an atomic row mutation.
CREATE TABLE IF NOT EXISTS apifull_bot_default_admin_rights (
    bot_user_id BIGINT PRIMARY KEY,
    group_admin_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    broadcast_admin_rights JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
