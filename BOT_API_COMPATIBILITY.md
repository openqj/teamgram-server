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
- Gifts and profile controls: `getAvailableGifts`, business-connection scoped
  gift conversion/upgrade/transfer, `setUserEmojiStatus`,
  `getUserProfileAudios`, `getCustomEmojiStickers`,
  `getForumTopicIconStickers`, and `getUserPersonalChatMessages` are routed
  through their Layer 229 adapters. Gift conversion, upgrade, and transfer
  enforce the corresponding business connection rights.
- Lifecycle and Mini App helpers: `savePreparedInlineMessage` and
  `savePreparedKeyboardButton` use Telegram's result contracts and the Layer
  229 prepared-inline/Mini App button services. `logOut` and `close` use the
  PostgreSQL `bot_api_lifecycle` table as an HTTP-only per-bot instance marker;
  they do not add Layer 229 fields or change MTProto session state. A missing
  lifecycle store returns a method-specific `501`.
  `deleteEphemeralMessage` and the four ephemeral edit methods use the native
  Layer 229 ephemeral-message service; receiver-only
  `ephemeral_message_parameters` sends for text, caption, media, and rich
  message methods use the same service. Photo, video, document, audio, and
  animation edits resolve through the existing DFS/media path.
- Poll and business controls: `stopPoll` preserves poll results while closing
  the poll in both ordinary message storage and the PostgreSQL channel store.
  Business account name, username, bio, and profile-photo methods enforce the
  connection rights before calling the corresponding Layer 229 services.
  `setBusinessAccountGiftSettings` updates only the gift privacy fields while
  preserving the owner's other global privacy settings; `getBusinessAccountStarBalance`
  requires the view-gifts-and-stars right and returns `StarAmount`.
- Message input accepts JSON, URL encoded forms, multipart uploads, existing
  Bot API file ids, UTF-16 entity offsets, inline keyboards, reply keyboards,
  force replies, Web App buttons, disabled link previews, empty inline keyboard
  removal, and HTTP(S) media URLs. Method names are case-insensitive, and
  message/caption whitespace is preserved.
- Message options preserve `allow_paid_broadcast` through the Layer 229
  `allow_paid_floodskip` flag. `message_effect_id` maps to the Layer 229
  `Effect` field for business, ordinary PostgreSQL outbox, media, rich, and
  forwarding paths. `show_caption_above_media` maps to the existing Layer 229
  `invert_media` flag for ordinary media and media-group sends.
- The Bot API response type supports Telegram's `parameters` envelope, and
  updates include forwarding origin, topic/offline/effect fields, inline callback queries,
  polls, poll answers, join requests, reactions, chat boosts, and paid-media
  purchase updates whenever the corresponding Layer 229 update is available.
- `botapi.ValidateWebAppInitData` implements Telegram's Web App HMAC contract
  for developer backends, including optional `auth_date` freshness checks.
- MTProto Layer 229 already contains the Mini App request and webview methods;
  BotFather menu buttons persist an HTTPS Mini App URL in the bot profile.

## Not yet complete

Compatibility snapshot (2026-10-18): the live Telegram Bot API documentation
exposes 185 official method names. All 185 are accepted case-insensitively by
the canonical HTTP route table. The table also keeps the legacy
`kickChatMember` alias for clients that still use Telegram's old method name.
Every official route has a dispatch adapter; an adapter can still return a
Telegram-shaped method-specific `501` when its configured backing service is
unavailable or a request field cannot be represented by the Layer 229 contract.
`sendGift`, `giftPremiumSubscription`, and `transferBusinessAccountStars` now
perform their Layer 229 payment-form validation and settlement when the payment
and Stars services are configured; `sendGift` accepts user and channel/group
targets when the underlying invoice service supports the target. Rich message
`html`, `markdown`, text
blocks, and photo/video/audio blocks are supported. Rich media blocks resolve
existing Bot API photo/document file ids into the corresponding Layer 229 input
vectors; upload and URL references remain invalid. Draft stop controls remain
explicitly unsupported.

The following contract details still differ from Telegram's current behavior:

- Formatting: `parse_mode` and `caption_parse_mode` support Telegram HTML,
  Markdown, and MarkdownV2 entity conversion. Explicit `entities`/
  `caption_entities` still take precedence, as in Telegram's API.
- Message methods: `stopPoll` and ephemeral text, caption, media, and reply
  markup edits are implemented. `suggested_post_parameters` validates the
  official Stars/TON price ranges and send-date window, then populates the
  Layer 229 `SuggestedPost` field on the direct message and media adapters.
  Requests that fall back to the PostgreSQL outbox return a method-specific
  `501` rather than dropping that field. Ephemeral
  `show_caption_above_media` and rich draft stop controls return `501` because
  the current Layer 229 records have no matching flags. Receiver-only
  `ephemeral_message_parameters` sends are supported; its
  `callback_query_id` and `replace_callback_query_message` controls return
  `501` because the current adapter has no corresponding TL fields.
  Ephemeral sends with `show_caption_above_media` or
  `link_preview_options.show_above_text` return `501` because the legacy
  ephemeral adapter drops those Layer 229 flags. `direct_messages_topic_id`
  returns `501` because the current message adapters do not expose an
  independent direct-topic field (values beyond the Layer 229 `int32` range
  are rejected with the same explicit status).
  `deleteMessages`,
  `deleteMessageReaction`, and `deleteAllMessageReactions` use the existing
  message and reaction services.
- Gifts: `getAvailableGifts` is complete. `sendGift` settles gifts for users
  and channel/group recipients through the Layer 229 Stars invoice form when
  the payment service supports the target. `sendGift` with
  `pay_for_upgrade=true` returns a method-specific `501` because the local
  payment adapter cannot represent the upgrade price safely. The Premium gift
  and business Stars transfer methods validate the official month/price and
  permission contracts before settling through the Layer 229 payment form.
  `upgradeGift` without a paid `star_count` is delegated to the Layer 229 gift
  provider; a positive `star_count` returns a method-specific `501` because
  the APIFull payment adapter does not yet support
  `inputInvoiceStarGiftUpgrade`.
  Gift conversion, upgrade, and transfer use the official
  `business_connection_id`/`owned_gift_id` shape and enforce business rights;
  numeric owned-gift IDs are scoped to the business owner and the underlying
  Layer 229 ledger still limits the supported gift filters and inventory
  representations described above. The singular Layer 229 saved-gift lookup
  accepts self-scoped chat references, including numeric Bot API
  `owned_gift_id` values; message-backed and non-self chat references remain
  unsupported, and chat/channel gift membership checks remain dependent on the
  target gift service.
- Chat administration: hidden member details and some channel-only
  administrative fields still need dedicated Bot API adapters.
- Lifecycle: the HTTP lifecycle marker blocks subsequent Bot API and file
  requests after `logOut` or `close`, clears pending Bot API updates on
  `logOut`, and prevents webhook delivery while inactive. It is a durable
  per-bot HTTP approximation: it does not implement Telegram's cloud/local
  server handoff, the ten-minute `close` launch window, webhook precondition,
  retry-after response, or token-version reset. An explicit administrative
  reset or token rotation policy is still required to reopen a bot.
- Inline mode and Mini Apps: external inline media is retained as WebDocument
  and is downloaded into DFS when an inline result is selected for sending;
  the remote URL is restricted to public HTTP(S) hosts and bounded to 20 MiB.
- Updates and webhooks: `my_chat_member` and `chat_member` are converted from
  channel/chat participant updates and persisted in the PostgreSQL Bot API
  queue. Webhook delivery uses the configured `max_connections` worker bound;
  failed HTTP requests remain queued and update the webhook error fields for a
  later retry. `removed_chat_boost` still depends on a matching Layer 229 update
  constructor being emitted by the source service.
- Bot profile commands: the default empty scope/language variant continues to
  use the existing `user.setBotCommands` MTProto contract. Other `scope` and
  `language_code` variants are persisted in PostgreSQL for the HTTP Bot API
  and follow Telegram's lookup fallback order. They are not visible through
  the current MTProto `user.getBotInfo` command source; chat usernames are
  stored as normalized HTTP keys and are not resolved to canonical peer
  access hashes.

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

## python-telegram-bot live acceptance

On 2026-10-18, `python-telegram-bot` 22.8 (upstream commit
`577e9c784607a42ff17746b9864b6416c3c30593`) ran against the isolated HTTP
server at `http://127.0.0.1:8801` without a client-side adapter. The PTB base
URLs were configured as `http://127.0.0.1:8801/bot{token}/` and
`http://127.0.0.1:8801/file/bot{token}/`.

The production-shaped probe completed 35/35 calls successfully: identity and
chat lookup, command management, administrator rights, webhook lifecycle,
inline and reply keyboards, message edits, ordinary-user chat actions, dice,
live-location edit/stop, venue, contact, poll/stop-poll, photo upload and
caption edit, multipart media groups, document upload/file download, Stars
invoice and invoice-link creation, filtered polling, and webhook set/get/delete.
The full multipart media-group path requires PTB's normal
`InputFile(..., attach=True)` references, exactly as Telegram's Bot API does.

This is runtime evidence for the exercised production paths, not a claim that
every Bot API method has identical backing behavior. The canonical route table
accepts all 185 official method names plus the legacy `kickChatMember` alias,
while the limitations in the section above still return explicit method-level
`501` responses until their Layer 229/service adapters exist.

## Runtime requirements

The HTTP server requires a reachable PostgreSQL 18 DSN and configured User,
Chat, Msg, BFF, and DFS RPC clients. Apply every PostgreSQL migration in
`teamgramd/deploy/sql/postgres` through the current head (including `103`,
`104`, and `105`) before starting the HTTP or sync services. The production
build now compiles and starts `httpserver` on port 8801; the container health
check requires that process to be running.
