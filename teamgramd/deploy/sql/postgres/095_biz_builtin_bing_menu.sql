-- Configure the built-in Bing bot's private-chat command menu.
INSERT INTO bot_commands (bot_id, command, description)
VALUES
    (900000000014, 'start', 'Show the Bing search menu'),
    (900000000014, 'help', 'Show Bing search help'),
    (900000000014, 'search', 'Search Bing images'),
    (900000000014, 'images', 'Search Bing images'),
    (900000000014, 'videos', 'Search Bing videos'),
    (900000000014, 'news', 'Search Bing news')
ON CONFLICT (bot_id, command) DO UPDATE SET description = EXCLUDED.description;

-- The standard Telegram menu button opens the command keyboard when no
-- custom Mini App menu button has been configured.
UPDATE bots
SET has_menu_button = FALSE,
    menu_button_type = 'commands',
    menu_button_text = '',
    menu_button_url = ''
WHERE bot_id = 900000000014;
