-- The MySQL message table was sharded by user, so random_id uniqueness was
-- scoped to each user's message view. Preserve that boundary in PostgreSQL.
DROP INDEX IF EXISTS messages_sender_random_key;
CREATE UNIQUE INDEX IF NOT EXISTS messages_user_sender_random_key
    ON messages (user_id, sender_user_id, random_id)
    WHERE random_id <> 0;
