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
  Mini App `web_app_query` updates.
- Files and messages: `getFile`, file download, `sendMessage`, `sendPhoto`,
  `sendDocument`, `sendVideo`, `sendVideoNote`, `sendSticker`, `sendAudio`, `sendVoice`, `sendAnimation`,
  `sendLocation`, `sendVenue`, `sendContact`, `sendPoll`, `sendDice`,
  `sendMediaGroup`, `forwardMessage`, `copyMessage`, `getChat`,
  `editMessageText`, `editMessageCaption`, `editMessageMedia`,
  `editMessageReplyMarkup`,
  `deleteMessage`, `setMessageReaction`, `answerCallbackQuery`, and
  `answerInlineQuery` for article text results.
  `answerWebAppQuery` supports
  `article` results whose `input_message_content` is text.
- Payment callbacks: `shipping_query` and `pre_checkout_query` updates,
  `answerShippingQuery`, and `answerPreCheckoutQuery` use the PostgreSQL Bot
  payment query state and preserve Telegram query expiry errors.
- Group administration: `getChatAdministrators`, `getChatMemberCount`,
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

- Remaining media and message methods: games, link preview preference fields
  beyond the disabled flag, and inline-message editing.
- Chat administration: chat/user profile photos, hidden member details, and
  some channel-only administrative fields still need dedicated Bot API
  adapters.
- Inline mode and Mini Apps: full inline result conversion and
  media/game/payment result variants for `answerInlineQuery` and
  `answerWebAppQuery`.
- Payments, invoices, shipping/pre-checkout callbacks, business connections,
  passport, boosts, and statistics.

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
`teamgramd/deploy/sql/postgres`, including migrations `080` through `087`,
before starting the HTTP or sync services. The production build now compiles
and starts `httpserver` on port 8801; the container health check requires that
process to be running.
