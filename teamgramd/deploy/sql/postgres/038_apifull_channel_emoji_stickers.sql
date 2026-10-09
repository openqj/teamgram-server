-- Keep channels.setEmojiStickers independent from the regular channel sticker set.
ALTER TABLE apifull_channel_sticker_set
    ADD CONSTRAINT apifull_channel_sticker_set_sticker_set_fk
    FOREIGN KEY (sticker_set_id) REFERENCES apifull_sticker_set(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS apifull_channel_emoji_sticker_set (
    channel_id BIGINT PRIMARY KEY,
    sticker_set_id BIGINT NOT NULL REFERENCES apifull_sticker_set(id) ON DELETE CASCADE,
    updated_by_user_id BIGINT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
