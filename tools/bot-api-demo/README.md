# Bot API developer demo

This is a real HTTP Bot API client. It does not call Teamgram internals. The
same shape can be copied into a developer's own bot service.

The one-shot smoke flow calls getMe, command management, webhook inspection,
menu button management when a Mini App URL is provided, and sends both Reply
Keyboard and Inline Keyboard messages when a chat is provided. It can also
upload a document, resolve a file id, verify Mini App initData, and poll
updates.

## Run

    export BOT_TOKEN='token returned by @BotFather'
    export BOT_CHAT_ID='private chat id or group id'
    export BOT_MINI_APP_URL='https://example.test/mini-app'
    ./tools/bot-api-e2e.sh -once

To verify webhook persistence and cleanup, use an HTTPS URL. The default flow
sets it, reads it back, and deletes it again:

    ./tools/bot-api-e2e.sh -webhook-url 'https://example.test/teamgram-bot'

Add -keep-webhook when the endpoint should remain configured. In that mode the
demo skips polling because Telegram does not allow getUpdates while a webhook
is active.

For a long-running developer bot:

    ./tools/bot-api-e2e.sh -poll -poll-timeout 25

Optional media checks:

    ./tools/bot-api-e2e.sh -chat-id "$BOT_CHAT_ID" -upload ./sample.pdf
    ./tools/bot-api-e2e.sh -chat-id "$BOT_CHAT_ID" -file-id 'document_<id>_<access-hash>'

PASS means the request reached the backend and returned ok: true. SKIP means
the request was intentionally omitted or the backend returned the documented
Telegram-shaped 501 for a method that is not implemented yet. FAIL means a
real request or response contract failed.
