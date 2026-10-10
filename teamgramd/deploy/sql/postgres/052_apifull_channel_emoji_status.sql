-- Durable Layer 229 channel emoji status fields.
ALTER TABLE apifull_channel
    ADD COLUMN IF NOT EXISTS emoji_status_document_id BIGINT NOT NULL DEFAULT 0;

ALTER TABLE apifull_channel
    ADD COLUMN IF NOT EXISTS emoji_status_until INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'apifull_channel_emoji_status_until_nonnegative'
          AND conrelid = 'apifull_channel'::regclass
    ) THEN
        ALTER TABLE apifull_channel
            ADD CONSTRAINT apifull_channel_emoji_status_until_nonnegative
            CHECK (emoji_status_until >= 0);
    END IF;
END
$$;
