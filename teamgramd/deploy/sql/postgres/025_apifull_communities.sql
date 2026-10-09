-- PostgreSQL 18 durable state for Layer 229 communities.
-- Community metadata is kept separate from the channel projection because a
-- community is a container of linked peers, while its wire representation is
-- the Layer 229 community Chat constructor.
CREATE TABLE IF NOT EXISTS apifull_community (
    community_id BIGINT PRIMARY KEY,
    owner_user_id BIGINT NOT NULL,
    title TEXT NOT NULL,
    about TEXT NOT NULL DEFAULT '',
    hidden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_apifull_community_owner
    ON apifull_community (owner_user_id, community_id);

CREATE TABLE IF NOT EXISTS apifull_community_peer (
    community_id BIGINT NOT NULL,
    peer_type SMALLINT NOT NULL,
    peer_id BIGINT NOT NULL,
    access_hash BIGINT NOT NULL DEFAULT 0,
    visible BOOLEAN,
    approved BOOLEAN NOT NULL DEFAULT TRUE,
    requested_by BIGINT NOT NULL DEFAULT 0,
    requested_at BIGINT NOT NULL DEFAULT 0,
    banned BOOLEAN NOT NULL DEFAULT FALSE,
    can_view_history BOOLEAN NOT NULL DEFAULT FALSE,
    PRIMARY KEY (community_id, peer_type, peer_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_community_peer_pending
    ON apifull_community_peer (community_id, approved, requested_at, peer_id);

CREATE TABLE IF NOT EXISTS apifull_community_dialog_state (
    user_id BIGINT NOT NULL,
    community_id BIGINT NOT NULL,
    collapsed BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, community_id)
);
