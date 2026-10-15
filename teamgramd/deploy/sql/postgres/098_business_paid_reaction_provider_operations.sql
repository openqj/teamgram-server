-- Register paid-reaction privacy methods in the isolated signed Business
-- provider fixture. Rows remain explicit provider authority; APIFull's local
-- PostgreSQL path is still selected when no Business provider is configured.
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
        'messages.getPaidReactionPrivacy',
        'messages.setBotGuestChatResult',
        'messages.setBotPrecheckoutResults',
        'messages.setBotShippingResults',
        'messages.requestChatJoinWebView',
        'messages.togglePaidReactionPrivacy'
    ));
