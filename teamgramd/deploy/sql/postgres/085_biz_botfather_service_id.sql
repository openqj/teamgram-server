-- Move legacy BotFather state away from the low isolated-fixture user id.
-- This is idempotent so a database that already ran the reserved-id seed is
-- unchanged, while databases that applied the first 080 revision are repaired
-- before the service starts.
INSERT INTO users (
    id, user_type, access_hash, first_name, last_name, username, phone,
    country_code, verified, support, is_bot, account_days_ttl, deleted
)
VALUES (
    900000000010, 3, 910000000010, 'BotFather', '', 'BotFather', 'botfather',
    'BOT', TRUE, TRUE, TRUE, 180, FALSE
)
ON CONFLICT (id) DO UPDATE SET
    user_type = EXCLUDED.user_type,
    access_hash = EXCLUDED.access_hash,
    first_name = EXCLUDED.first_name,
    last_name = EXCLUDED.last_name,
    username = EXCLUDED.username,
    phone = EXCLUDED.phone,
    country_code = EXCLUDED.country_code,
    verified = EXCLUDED.verified,
    support = EXCLUDED.support,
    is_bot = EXCLUDED.is_bot,
    deleted = FALSE;

INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
VALUES ('BotFather', 2, 900000000010, FALSE, TRUE, 0, FALSE)
ON CONFLICT DO NOTHING;

UPDATE username
SET peer_type = 2,
    peer_id = 900000000010,
    editable = FALSE,
    active = TRUE,
    deleted = FALSE
WHERE lower(username) = lower('BotFather');

INSERT INTO bots (bot_id, creator_user_id, manager_bot_id, token, description)
VALUES (900000000010, 0, 0, '', 'Manage your bots')
ON CONFLICT (bot_id) DO UPDATE SET
    description = EXCLUDED.description;

INSERT INTO botfather_sessions (user_id, state, pending_name, pending_bot_id, last_message_id, last_reply, updated_at)
SELECT 900000000010, state, pending_name, pending_bot_id, last_message_id, last_reply, updated_at
FROM botfather_sessions
WHERE user_id = 6
ON CONFLICT (user_id) DO UPDATE SET
    state = EXCLUDED.state,
    pending_name = EXCLUDED.pending_name,
    pending_bot_id = EXCLUDED.pending_bot_id,
    last_message_id = EXCLUDED.last_message_id,
    last_reply = EXCLUDED.last_reply,
    updated_at = EXCLUDED.updated_at;

DELETE FROM botfather_sessions WHERE user_id = 6;
DELETE FROM bots
WHERE bot_id = 6
  AND EXISTS (SELECT 1 FROM users WHERE id = 6 AND lower(COALESCE(username, '')) = lower('BotFather'));

-- Only retire the legacy service row. An isolated fixture user with id 6 is
-- preserved, including its username and messages.
UPDATE users
SET deleted = TRUE,
    username = NULL,
    phone = NULL,
    is_bot = FALSE,
    support = FALSE
WHERE id = 6 AND lower(COALESCE(username, '')) = lower('BotFather');

SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    GREATEST(COALESCE((SELECT MAX(id) FROM users), 1), 1),
    TRUE
);
