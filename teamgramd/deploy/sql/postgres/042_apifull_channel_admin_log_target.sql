-- Support takeout channels.getLeftChannels latest-event lookup.
CREATE INDEX IF NOT EXISTS idx_apifull_channel_admin_log_target
    ON apifull_channel_admin_log (target_user_id, channel_id, id DESC);
