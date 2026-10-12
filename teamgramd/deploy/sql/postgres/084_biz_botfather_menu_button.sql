ALTER TABLE botfather_sessions
    ADD COLUMN IF NOT EXISTS pending_menu_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pending_menu_url TEXT NOT NULL DEFAULT '';
