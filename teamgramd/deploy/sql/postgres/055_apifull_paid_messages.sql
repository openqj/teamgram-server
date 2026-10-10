-- Durable channels.updatePaidMessagesPrice state.
ALTER TABLE apifull_channel
    ADD COLUMN IF NOT EXISTS broadcast_messages_allowed SMALLINT NOT NULL DEFAULT 0;

ALTER TABLE apifull_channel
    ADD COLUMN IF NOT EXISTS send_paid_messages_stars BIGINT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'apifull_channel_paid_messages_stars_nonnegative'
          AND conrelid = 'apifull_channel'::regclass
    ) THEN
        ALTER TABLE apifull_channel
            ADD CONSTRAINT apifull_channel_paid_messages_stars_nonnegative
            CHECK (send_paid_messages_stars IS NULL OR send_paid_messages_stars >= 0);
    END IF;
END
$$;
