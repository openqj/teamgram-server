-- Extend the isolated signed provider control plane for message-side Bot APIs.
-- PostgreSQL payment callbacks remain authoritative when APIFull is enabled;
-- these names only make the provider fallback explicit for isolated fixtures.
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
        'messages.setBotGuestChatResult',
        'messages.requestChatJoinWebView',
        'messages.setBotShippingResults',
        'messages.setBotPrecheckoutResults'
    ));
