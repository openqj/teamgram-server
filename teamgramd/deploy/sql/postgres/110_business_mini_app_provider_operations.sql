-- Allow the signed external Business provider to authorize the remaining
-- Mini App operations. Rows are still explicit and keyed by the exact
-- request fingerprint in teamgram_business_bot_operation.
ALTER TABLE teamgram_business_bot_operation
    DROP CONSTRAINT IF EXISTS teamgram_business_bot_operation_operation_check;

ALTER TABLE teamgram_business_bot_operation
    ADD CONSTRAINT teamgram_business_bot_operation_operation_check
    CHECK (operation IN (
        'bots.answerWebhookJSONQuery',
        'bots.checkDownloadFileParams',
        'bots.getBotRecommendations',
        'bots.getPopularAppBots',
        'bots.invokeWebViewCustomMethod',
        'bots.sendCustomRequest',
        'bots.setCustomVerification',
        'bots.setJoinChatResults',
        'bots.updateStarRefProgram',
        'messages.editInlineBotMessage',
        'messages.getBotApp',
        'messages.getPaidReactionPrivacy',
        'messages.requestAppWebView',
        'messages.requestChatJoinWebView',
        'messages.sendWebViewData',
        'messages.sendWebViewResultMessage',
        'messages.setBotGuestChatResult',
        'messages.setBotPrecheckoutResults',
        'messages.setBotShippingResults',
        'messages.togglePaidReactionPrivacy'
    ));
