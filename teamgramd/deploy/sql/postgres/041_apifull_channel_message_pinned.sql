-- Add the channel message pin state without changing an already-applied migration checksum.
ALTER TABLE apifull_channel_message
    ADD COLUMN IF NOT EXISTS pinned SMALLINT NOT NULL DEFAULT 0;
