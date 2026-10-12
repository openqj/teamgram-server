-- BotFather is a first-class bot account. Its messages are consumed by the
-- dedicated BotFather topic and handled by messenger.sync.
CREATE TABLE IF NOT EXISTS botfather_sessions (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    state TEXT NOT NULL DEFAULT 'idle',
    pending_name TEXT NOT NULL DEFAULT '',
    pending_bot_id BIGINT NOT NULL DEFAULT 0,
    last_message_id INTEGER NOT NULL DEFAULT 0,
    last_reply TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

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

UPDATE username
SET peer_type = 2,
    peer_id = 900000000010,
    editable = FALSE,
    active = TRUE,
    deleted = FALSE
WHERE lower(username) = lower('BotFather');

INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
SELECT 'BotFather', 2, 900000000010, FALSE, TRUE, 0, FALSE
WHERE NOT EXISTS (
    SELECT 1 FROM username
    WHERE lower(username) = lower('BotFather') AND deleted = FALSE
);

INSERT INTO bots (bot_id, creator_user_id, manager_bot_id, token, description)
VALUES (900000000010, 0, 0, '', 'Manage your bots')
ON CONFLICT (bot_id) DO UPDATE SET
    description = EXCLUDED.description;

-- Explicit service-account seeds must not leave the users identity sequence
-- behind the highest existing user id. This keeps subsequent regular and bot
-- accounts collision-free on both fresh and already-seeded databases.
SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    GREATEST(COALESCE((SELECT MAX(id) FROM users), 1), 1),
    TRUE
);
