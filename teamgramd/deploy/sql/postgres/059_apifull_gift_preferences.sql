-- PostgreSQL 18 durable preferences for saved Star Gifts.
-- Both tables are owner-scoped. The gift pin table references the
-- authoritative APIFull inventory so deleting a gift cannot leave a pin.
CREATE TABLE IF NOT EXISTS apifull_gift_notification_setting (
    owner_user_id BIGINT NOT NULL,
    peer_type SMALLINT NOT NULL,
    peer_id BIGINT NOT NULL,
    enabled BOOLEAN NOT NULL,
    updated_at BIGINT NOT NULL,
    PRIMARY KEY (owner_user_id, peer_type, peer_id),
    CONSTRAINT apifull_gift_notification_peer_type CHECK (peer_type IN (1, 2, 3, 4)),
    CONSTRAINT apifull_gift_notification_peer_id CHECK (peer_id > 0)
);

CREATE TABLE IF NOT EXISTS apifull_gift_pin (
    owner_user_id BIGINT NOT NULL,
    gift_id BIGINT NOT NULL REFERENCES apifull_gift(id) ON DELETE CASCADE,
    position INTEGER NOT NULL,
    PRIMARY KEY (owner_user_id, gift_id),
    CONSTRAINT apifull_gift_pin_position CHECK (position >= 0),
    CONSTRAINT apifull_gift_pin_order UNIQUE (owner_user_id, position)
);

CREATE INDEX IF NOT EXISTS idx_apifull_gift_pin_owner_order
    ON apifull_gift_pin (owner_user_id, position, gift_id);
