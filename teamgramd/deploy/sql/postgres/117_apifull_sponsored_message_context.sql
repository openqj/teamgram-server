-- Add the optional Layer 229 message selector to sponsored-message inventory.
-- A message-scoped request must never fall back to an unrelated peer-wide ad.
ALTER TABLE apifull_sponsored_inventory
    ADD COLUMN IF NOT EXISTS message_id INTEGER NOT NULL DEFAULT 0;

ALTER TABLE apifull_sponsored_inventory
    DROP CONSTRAINT IF EXISTS apifull_sponsored_inventory_message_context;

ALTER TABLE apifull_sponsored_inventory
    ADD CONSTRAINT apifull_sponsored_inventory_message_context
        CHECK (message_id >= 0 AND (inventory_type = 'messages' OR message_id = 0));

DROP INDEX IF EXISTS apifull_sponsored_inventory_messages_key;

CREATE UNIQUE INDEX IF NOT EXISTS apifull_sponsored_inventory_messages_key
    ON apifull_sponsored_inventory (inventory_type, peer_key, message_id)
    WHERE inventory_type = 'messages';
