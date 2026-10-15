-- Telegram chatInvitePeek grants temporary read access to a private
-- supergroup or channel after messages.checkChatInvite succeeds.
CREATE TABLE IF NOT EXISTS apifull_channel_peek_access (
    channel_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    expires_at BIGINT NOT NULL CHECK (expires_at > 0),
    granted_at BIGINT NOT NULL,
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_apifull_channel_peek_access_expiry
    ON apifull_channel_peek_access (user_id, expires_at);
