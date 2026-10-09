-- Internal service messages may have random_id = 0. Retain their delivery
-- identity after deletion so consumer retries cannot recreate message views.
CREATE UNIQUE INDEX IF NOT EXISTS messages_user_sender_delivery_key
    ON messages (user_id, sender_user_id, dialog_message_id)
    WHERE dialog_message_id <> 0;
