-- PostgreSQL 18 monotonic discussion read cursors.
-- The key is scoped to the caller, peer and discussion root so reads from
-- different users or threads cannot overwrite one another.
CREATE TABLE IF NOT EXISTS apifull_discussion_read_state (
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    peer_type INTEGER NOT NULL CHECK (peer_type > 0),
    peer_id BIGINT NOT NULL CHECK (peer_id > 0),
    message_id INTEGER NOT NULL CHECK (message_id > 0),
    read_max_id INTEGER NOT NULL CHECK (read_max_id >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, peer_type, peer_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_discussion_read_state_user
    ON apifull_discussion_read_state (user_id, updated_at DESC);
