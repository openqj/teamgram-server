# Bot API compatibility

The server exposes a Telegram-shaped Bot API at:

```text
POST /bot<TOKEN>/<method>
GET  /bot<TOKEN>/<method>
GET  /file/bot<TOKEN>/<file_path>
```

The transitional `/bot/<TOKEN>/<method>` and `/file/<TOKEN>/<file_path>`
forms remain accepted for existing deployments.

Bot tokens are issued by the built-in `@BotFather` account. A user can send
`/newbot`, choose a display name and a username ending in `bot`, then use the
returned token with the HTTP API. BotFather state and the Bot API update queue
are stored in PostgreSQL.

## Implemented

- BotFather: `/newbot`, `/mybots`, `/token`, `/revoke`, `/setname`,
  `/setdescription`, `/setabouttext`, `/setcommands`, `/setprivacy`,
  `/setjoingroups`, `/setinline`, `/setinlinegeo`, `/setmenubutton`,
  `/newapp`, `/myapps`, `/editapp`, `/deleteapp`, `/setdomain`, and
  `/cancel`. Mini Apps are stored in PostgreSQL; the selected main app URL is
  also reflected in `bots.main_app_url` for MTProto
  `messages.requestMainWebView`.
- Authentication and identity: `getMe`, token validation, bot profile fields,
  command management, and menu button management.
- Updates: `getUpdates` with offset, limit, timeout, and allowed update
  filtering; `setWebhook`, `deleteWebhook`, `getWebhookInfo`, secret-token
  delivery, pending update draining, inline queries, callback queries, and
  `chosen_inline_result`, Mini App `web_app_query`, and business updates.
- Inline result responses preserve Telegram's `cache_time`, `is_personal`,
  gallery, pagination offset, switch-to-private-chat button, and Mini App
  button through the durable PostgreSQL hand-off. Cached photo, GIF, MPEG-4
  GIF, video, audio, voice, document, and sticker result constructors retain
  their canonical MTProto media type.
- Files and messages: `getFile`, file download, `sendMessage`, `sendPhoto`,
  `sendDocument`, `sendVideo`, `sendVideoNote`, `sendSticker`, `sendAudio`, `sendVoice`, `sendAnimation`,
  `sendLocation`, `sendVenue`, `sendContact`, `sendPoll`, `sendDice`,
  `sendMediaGroup`, `sendChatAction`, `sendGame`, `sendInvoice`,
  `createInvoiceLink`, `getMyStarBalance`, `getStarTransactions`,
  `refundStarPayment`, `editUserStarSubscription`,
  `forwardMessage`, `copyMessage`, `getChat`,
  `editMessageText`, `editMessageCaption`, `editMessageMedia`,
  `editMessageReplyMarkup`, `setGameScore`, `getGameHighScores`,
  `deleteMessage`, `setMessageReaction`, `answerCallbackQuery`, and
  `answerInlineQuery` for article, media, cached media, location, venue,
  contact, and game results.
  Payment completion service messages expose `successful_payment` with order,
  payload, provider charge, Telegram charge, and recurring subscription fields.
  `answerWebAppQuery` supports article, media, cached media, location, venue,
  contact, and game result constructors with the same validation as inline
  answers.
- Payment callbacks: `shipping_query` and `pre_checkout_query` updates,
  `answerShippingQuery`, and `answerPreCheckoutQuery` use the PostgreSQL Bot
  payment query state and preserve Telegram query expiry errors.
- Business connections: `getBusinessConnection` reads the Layer 229
  `account.getBotBusinessConnection` update and returns the Bot API
  `BusinessConnection` object. Business connection, new/edit business
  message, deleted business message, and business callback updates are
  serialized with their `business_connection_id`. The PostgreSQL Bot API
  update queue hydrates the connected user's authoritative profile before
  serializing business connection updates.
- Group administration: `getMyDefaultAdministratorRights`,
  `setMyDefaultAdministratorRights`, `getChatAdministrators`, `getChatMemberCount`,
  `getChatMember`, `banChatMember`, `unbanChatMember`,
  `restrictChatMember`, `promoteChatMember`, `setChatPermissions`,
  `setChatTitle`, `setChatDescription`, pin/unpin methods, and `leaveChat`
  use the existing Chat/Channels MTProto methods and preserve Telegram member
  status and permission fields.
- Invite links and join requests: `exportChatInviteLink`,
  `createChatInviteLink`, `editChatInviteLink`, `revokeChatInviteLink`,
  `approveChatJoinRequest`, and `declineChatJoinRequest` use the Layer 229
  `RPCChatInvites` service. Invite edits unwrap both exported-invite union
  constructors and return Telegram `ChatInviteLink` fields.
- Forum topics: `createForumTopic`, `closeForumTopic`, `reopenForumTopic`,
  `editForumTopic`, `deleteForumTopic`, `unpinAllForumTopicMessages`,
  `hideGeneralForumTopic`, and `unhideGeneralForumTopic` use `RPCForums` and
  `RPCMessages`. Topic creation reads the committed PostgreSQL topic back so
  the returned `message_thread_id` is the persisted Layer 229 topic id.
- Profile photos: `getUserProfilePhotos`, `setChatPhoto`, and
  `deleteChatPhoto` use the Layer 229 photo/chat RPCs and DFS upload path.
- Message input accepts JSON, URL encoded forms, multipart uploads, existing
  Bot API file ids, UTF-16 entity offsets, inline keyboards, reply keyboards,
  force replies, Web App buttons, disabled link previews, and empty inline
  keyboard removal.
- `botapi.ValidateWebAppInitData` implements Telegram's Web App HMAC contract
  for developer backends, including optional `auth_date` freshness checks.
- MTProto Layer 229 already contains the Mini App request and webview methods;
  BotFather menu buttons persist an HTTPS Mini App URL in the bot profile.

## Not yet complete

The following method groups still need dedicated protocol adapters before the
server can claim full current Telegram Bot API compatibility:

- Remaining media and message methods: link preview preference fields beyond
  the disabled and show-above-text flags.
- Chat administration: hidden member details and some channel-only
  administrative fields still need dedicated Bot API adapters.
- Inline mode and Mini Apps: external inline media is retained as WebDocument
  and is downloaded into DFS when an inline result is selected for sending;
  the remote URL is restricted to public HTTP(S) hosts and bounded to 20 MiB.
- Business message actions: `readBusinessMessage` and
  `deleteBusinessMessages`, plus business-scoped message editing, still need
  a generated Layer 229 adapter that carries `invokeWithBusinessConnection`
  through the service boundary.
- Payments, invoices, passport, boosts, and statistics still have provider or
  Bot API adapter gaps beyond invoice delivery, invoice links, Stars account
  balance/history/refunds/subscription edits, and shipping/pre-checkout
  callbacks.

Unsupported methods return a Telegram-shaped `501` response rather than a
successful placeholder. New methods must preserve the existing MTProto Layer
229 constructors, PostgreSQL transaction boundaries, update ordering, and
canonical RPC errors.

## Developer acceptance client

`tools/bot-api-demo` is a real public-HTTP Bot API client. It exercises
`getMe`, command management, webhook set/get/delete, Mini App menu buttons,
Reply Keyboard, Inline Keyboard, callbacks, inline results, Web App results,
payment query acknowledgements, multipart document upload, file lookup, and
long polling when the corresponding event or option is supplied.

Run it with a token returned by `@BotFather`:

    BOT_TOKEN='...' BOT_CHAT_ID='...' BOT_MINI_APP_URL='https://example.test/app' \
      ./tools/bot-api-e2e.sh -once

`PASS` is a real `ok: true` response, `SKIP` is an omitted option or a
documented 501 method, and `FAIL` is a contract or runtime failure. See
`tools/bot-api-demo/README.md` for webhook and long-polling examples.

## Runtime requirements

The HTTP server requires a reachable PostgreSQL 18 DSN and configured User,
Chat, Msg, BFF, and DFS RPC clients. Apply the PostgreSQL migrations through
`teamgramd/deploy/sql/postgres`, including migrations `080` through `090`,
before starting the HTTP or sync services. The production build now compiles
and starts `httpserver` on port 8801; the container health check requires that
process to be running.
