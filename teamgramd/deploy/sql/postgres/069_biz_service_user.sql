-- Seed the Telegram service account used as the sender for login-code messages.
-- The account must be a live PostgreSQL user so the inbox consumer can resolve
-- the sender while delivering auth.sendCode updates.
INSERT INTO users (
    id,
    user_type,
    access_hash,
    secret_key_id,
    first_name,
    last_name,
    username,
    phone,
    country_code,
    verified,
    support,
    is_bot,
    account_days_ttl,
    deleted
)
VALUES (
    777000,
    4,
    6599886787491911851,
    6895602324158323006,
    'Teamgram',
    '',
    'teamgram',
    '42777',
    '',
    TRUE,
    FALSE,
    FALSE,
    180,
    FALSE
)
ON CONFLICT (id) DO UPDATE SET
    user_type = EXCLUDED.user_type,
    access_hash = EXCLUDED.access_hash,
    secret_key_id = EXCLUDED.secret_key_id,
    first_name = EXCLUDED.first_name,
    username = EXCLUDED.username,
    phone = EXCLUDED.phone,
    verified = EXCLUDED.verified,
    support = EXCLUDED.support,
    is_bot = EXCLUDED.is_bot,
    deleted = FALSE;
