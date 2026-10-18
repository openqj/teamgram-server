-- Make the built-in support peer a standard Bot so clients expose its command
-- menu and reply keyboard controls.
UPDATE users
SET user_type = 3,
    is_bot = TRUE,
    phone = 'volunteer-support',
    country_code = 'BOT',
    deleted = FALSE
WHERE id = 900000000016;

INSERT INTO bots (bot_id, creator_user_id, manager_bot_id, token, description)
VALUES (900000000016, 0, 0, 'builtin-volunteer-support', 'Volunteer support intake')
ON CONFLICT (bot_id) DO UPDATE SET
    description = EXCLUDED.description,
    token = EXCLUDED.token;

INSERT INTO bot_commands (bot_id, command, description)
VALUES
    (900000000016, 'start', 'Start a support request'),
    (900000000016, 'new', 'Start a new support request'),
    (900000000016, 'cancel', 'Cancel the current support request')
ON CONFLICT (bot_id, command) DO UPDATE SET description = EXCLUDED.description;
