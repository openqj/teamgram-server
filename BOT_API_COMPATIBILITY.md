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
  `sendRichMessage`, `sendRichMessageDraft`,
  `sendMediaGroup`, `sendChatAction`, `sendGame`, `sendInvoice`,
  `createInvoiceLink`, `getMyStarBalance`, `getStarTransactions`,
  `refundStarPayment`, `editUserStarSubscription`,
  `forwardMessage`, `copyMessage`, `forwardMessages`, `copyMessages`, `getChat`,
  `editMessageText`, `editMessageCaption`, `editMessageMedia`,
  `editMessageReplyMarkup`, `editMessageLiveLocation`,
  `stopMessageLiveLocation`, `setGameScore`, `getGameHighScores`,
  `deleteMessage`, `deleteMessages`, `deleteMessageReaction`,
  `deleteAllMessageReactions`, `setMessageReaction`, `answerCallbackQuery`, and
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
  `setBusinessAccountProfilePhoto` uses the connected business user's edit
  permission, bot-authenticated DFS upload, and owner-scoped
  `photos.uploadProfilePhoto`.
- Sticker sets: `getStickerSet`, `createNewStickerSet`, `addStickerToSet`,
  position/delete/replace operations, emoji/keyword/mask edits, thumbnail and
  title changes, custom emoji thumbnails, and `deleteStickerSet` use the Layer
  229 sticker services. `setChatStickerSet` and `deleteChatStickerSet` use
  `channels.setStickers` for supergroups.
- Gifts and profile controls: `getAvailableGifts`, local-owner gift
  conversion/upgrade/transfer, `setUserEmojiStatus`,
  `getUserProfileAudios`, `getCustomEmojiStickers`,
  `getForumTopicIconStickers`, and `getUserPersonalChatMessages` are routed
  through their Layer 229 adapters. Gift mutation is limited to the bot's
  local owner ledger until business-account gift scope is available.
- Lifecycle and Mini App helpers: `logOut`, `close`, and
  `savePreparedInlineMessage` use Telegram's boolean/result contracts and the
  Layer 229 prepared-inline persistence service. `deleteEphemeralMessage` and
  the four ephemeral edit methods use the native Layer 229 ephemeral-message
  service; photo, video, document, audio, and animation edits resolve through
  the existing DFS/media path.
- Poll and business controls: `stopPoll` preserves poll results while closing
  the poll in both ordinary message storage and the PostgreSQL channel store.
  Business account name, username, bio, and profile-photo methods enforce the
  connection rights before calling the corresponding Layer 229 services.
- Message input accepts JSON, URL encoded forms, multipart uploads, existing
  Bot API file ids, UTF-16 entity offsets, inline keyboards, reply keyboards,
  force replies, Web App buttons, disabled link previews, empty inline keyboard
  removal, and HTTP(S) media URLs. Method names are case-insensitive, and
  message/caption whitespace is preserved.
- The Bot API response type supports Telegram's `parameters` envelope, and
  updates include forwarding origin, topic/offline/effect fields, inline callback queries,
  polls, poll answers, join requests, reactions, chat boosts, and paid-media
  purchase updates whenever the corresponding Layer 229 update is available.
- `botapi.ValidateWebAppInitData` implements Telegram's Web App HMAC contract
  for developer backends, including optional `auth_date` freshness checks.
- MTProto Layer 229 already contains the Mini App request and webview methods;
  BotFather menu buttons persist an HTTPS Mini App URL in the bot profile.

## Not yet complete

Compatibility snapshot (2026-10-17): the live Telegram documentation exposes
183 official method names. All 183 are accepted case-insensitively by the
canonical HTTP route table, which also keeps the project extensions
`sendRichMessage`/`sendRichMessageDraft` and the legacy `kickChatMember` alias.
178 official methods have concrete dispatch adapters. Five route names return a
Telegram-shaped method-specific `501` because their Layer 229/service contract
is not available: `sendGift`, `giftPremiumSubscription`,
`setBusinessAccountGiftSettings`, `transferBusinessAccountStars`, and
`savePreparedKeyboardButton`. Other dispatched methods can still return `501`
when their configured backing service is unavailable. Rich message `html`,
`markdown`, and text-block requests are supported; rich media blocks and draft
stop controls remain explicitly unsupported.

The following contract details still differ from Telegram's current behavior:

- Formatting: `parse_mode` and `caption_parse_mode` support Telegram HTML,
  Markdown, and MarkdownV2 entity conversion. Explicit `entities`/
  `caption_entities` still take precedence, as in Telegram's API.
- Message methods: `stopPoll` and ephemeral text, caption, media, and reply
  markup edits are implemented. Ephemeral `show_caption_above_media` and rich
  draft stop controls return a method-specific `501` because the current
  Layer 229 records have no matching flags. `deleteMessages`,
  `deleteMessageReaction`, and `deleteAllMessageReactions` use the existing
  message and reaction services.
- Gifts: `getAvailableGifts` is complete. Gift conversion, upgrade, and
  transfer currently accept the project's local `user_id`/`owned_gift_id`
  ledger shape and reject non-owner accounts or positive `star_count`; the
  current official `business_connection_id` flow still needs a business gift
  adapter. `sendGift` and `giftPremiumSubscription` remain unsupported.
- Chat administration: hidden member details and some channel-only
  administrative fields still need dedicated Bot API adapters.
- Inline mode and Mini Apps: external inline media is retained as WebDocument
  and is downloaded into DFS when an inline result is selected for sending;
  the remote URL is restricted to public HTTP(S) hosts and bounded to 20 MiB.
- Updates and webhooks: `my_chat_member` and `chat_member` are converted from
  channel/chat participant updates and persisted in the PostgreSQL Bot API
  queue. Webhook delivery uses the configured `max_connections` worker bound;
  failed HTTP requests remain queued and update the webhook error fields for a
  later retry. `removed_chat_boost` still depends on a matching Layer 229 update
  constructor being emitted by the source service.
- Bot profile commands: `setMyCommands` and `getMyCommands` currently use the
  default command scope and language only; Telegram's `scope` and
  `language_code` variants need persistence support.

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
