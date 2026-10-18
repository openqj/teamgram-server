-- Allow the business provider to authorize the non-paid Star Gift transfer
-- that follows a Bot API paid transfer settlement.
ALTER TABLE teamgram_business_payment_operation
    DROP CONSTRAINT IF EXISTS teamgram_business_payment_operation_operation_check;

ALTER TABLE teamgram_business_payment_operation
    ADD CONSTRAINT teamgram_business_payment_operation_operation_check
    CHECK (operation IN (
        'payments.launchPrepaidGiveaway',
        'payments.getStarGiftUpgradePreview',
        'payments.upgradeStarGift',
        'payments.transferStarGift',
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
    ));
