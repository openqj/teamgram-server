-- Explicit rows for the isolated signed gift and giveaway provider fixture.
-- A row is usable only for the exact operation, caller and request payload
-- fingerprint provisioned by an acceptance run.
CREATE TABLE IF NOT EXISTS teamgram_business_payment_operation (
    operation TEXT NOT NULL CHECK (operation IN (
        'payments.launchPrepaidGiveaway',
        'payments.getStarGiftUpgradePreview',
        'payments.upgradeStarGift',
        'payments.getStarGiftWithdrawalUrl',
        'payments.getResaleStarGifts',
        'payments.updateStarGiftPrice',
        'payments.checkCanSendGift',
        'payments.getStarGiftAuctionState',
        'payments.getStarGiftAuctionAcquiredGifts',
        'payments.getStarGiftActiveAuctions',
        'payments.resolveStarGiftOffer',
        'payments.sendStarGiftOffer',
        'payments.getStarGiftUpgradeAttributes',
        'payments.getCraftStarGifts',
        'payments.craftStarGift'
    )),
    user_id BIGINT NOT NULL CHECK (user_id > 0),
    request_fingerprint BYTEA NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    result JSONB NOT NULL CHECK (jsonb_typeof(result) = 'object'),
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (operation, user_id, request_fingerprint)
);

CREATE INDEX IF NOT EXISTS idx_teamgram_business_payment_operation_expiry
    ON teamgram_business_payment_operation (expires_at)
    WHERE expires_at IS NOT NULL;
