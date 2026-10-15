-- Register the built-in inline search bots used by the web client examples.
-- The high IDs are reserved service identities and stay above the user
-- identity sequence used by the fresh PostgreSQL deployment.

INSERT INTO users (
    id, user_type, access_hash, first_name, last_name, username, phone,
    country_code, verified, support, is_bot, account_days_ttl, about, deleted
)
VALUES
    (900000000011, 3, 910000000011, 'GIF Search', '', 'gif', 'teamgram-inline-gif', 'BOT', TRUE, FALSE, TRUE, 180, 'Search GIFs', FALSE),
    (900000000012, 3, 910000000012, 'Video Search', '', 'vid', 'teamgram-inline-vid', 'BOT', TRUE, FALSE, TRUE, 180, 'Search videos', FALSE),
    (900000000013, 3, 910000000013, 'Yandex Images', '', 'pic', 'teamgram-inline-pic', 'BOT', TRUE, FALSE, TRUE, 180, 'Search Yandex images', FALSE),
    (900000000014, 3, 910000000014, 'Bing Images', '', 'bing', 'teamgram-inline-bing', 'BOT', TRUE, FALSE, TRUE, 180, 'Search Bing images', FALSE),
    (900000000015, 3, 910000000015, 'Wikipedia', '', 'wiki', 'teamgram-inline-wiki', 'BOT', TRUE, FALSE, TRUE, 180, 'Search Wikipedia', FALSE)
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
    account_days_ttl = EXCLUDED.account_days_ttl,
    about = EXCLUDED.about,
    deleted = FALSE;

INSERT INTO username (username, peer_type, peer_id, editable, active, order2, deleted)
VALUES
    ('gif', 2, 900000000011, FALSE, TRUE, 0, FALSE),
    ('vid', 2, 900000000012, FALSE, TRUE, 0, FALSE),
    ('pic', 2, 900000000013, FALSE, TRUE, 0, FALSE),
    ('bing', 2, 900000000014, FALSE, TRUE, 0, FALSE),
    ('wiki', 2, 900000000015, FALSE, TRUE, 0, FALSE)
ON CONFLICT DO NOTHING;

UPDATE username SET peer_type = 2, peer_id = 900000000011, editable = FALSE, active = TRUE, deleted = FALSE WHERE lower(username) = 'gif';
UPDATE username SET peer_type = 2, peer_id = 900000000012, editable = FALSE, active = TRUE, deleted = FALSE WHERE lower(username) = 'vid';
UPDATE username SET peer_type = 2, peer_id = 900000000013, editable = FALSE, active = TRUE, deleted = FALSE WHERE lower(username) = 'pic';
UPDATE username SET peer_type = 2, peer_id = 900000000014, editable = FALSE, active = TRUE, deleted = FALSE WHERE lower(username) = 'bing';
UPDATE username SET peer_type = 2, peer_id = 900000000015, editable = FALSE, active = TRUE, deleted = FALSE WHERE lower(username) = 'wiki';

INSERT INTO bots (bot_id, creator_user_id, manager_bot_id, token, description, bot_inline_placeholder)
VALUES
    (900000000011, 900000000010, 0, 'teamgram-inline-gif', 'Search GIFs', 'Search GIFs'),
    (900000000012, 900000000010, 0, 'teamgram-inline-vid', 'Search videos', 'Search videos'),
    (900000000013, 900000000010, 0, 'teamgram-inline-pic', 'Search Yandex images', 'Search Yandex images'),
    (900000000014, 900000000010, 0, 'teamgram-inline-bing', 'Search Bing images', 'Search Bing images'),
    (900000000015, 900000000010, 0, 'teamgram-inline-wiki', 'Search Wikipedia', 'Search Wikipedia')
ON CONFLICT (bot_id) DO UPDATE SET
    creator_user_id = EXCLUDED.creator_user_id,
    manager_bot_id = EXCLUDED.manager_bot_id,
    token = EXCLUDED.token,
    description = EXCLUDED.description,
    bot_inline_placeholder = EXCLUDED.bot_inline_placeholder;

SELECT setval(
    pg_get_serial_sequence('users', 'id'),
    GREATEST(COALESCE((SELECT MAX(id) FROM users), 1), 1),
    TRUE
);
