# PTB Bot Example

This example uses the upstream `python-telegram-bot` API directly. It handles
`/start`, ordinary text messages, a persistent reply keyboard, inline keyboard
callbacks, a paginated catalog of all 185 official methods, and a full PTB
smoke command. Methods supported by the installed PTB release use their typed
method; newer methods use PTB's `Bot.do_api_request` escape hatch.

## Run against Teamgram

Use the token issued by `@BotFather` and keep the `{token}` placeholder in both
base URLs:

```sh
python3 -m venv .venv
.venv/bin/pip install python-telegram-bot==22.8
BOT_TOKEN='...' \
BOT_API_BASE_URL='http://127.0.0.1:8801/bot{token}/' \
BOT_API_FILE_URL='http://127.0.0.1:8801/file/bot{token}/' \
.venv/bin/python tools/ptb-bot/main.py
```

Open the Teamgram Web client and send `/start` to the bot. The bot enables a
persistent keyboard below the composer (`Keyboard ping`, `Help`) and sends a
separate inline controls message with `Ping bot`. Set `BOT_MINI_APP_URL` to add
an `Open Mini App` reply-keyboard button. The PTB handler receives the resulting
`web_app_data` update and echoes its data. Clicking `Keyboard ping` sends a
normal text update; clicking `Ping bot` sends a callback query and edits the
inline controls message. Any ordinary text is echoed with the persistent
keyboard and a fresh inline controls message.

Send `/methods` to browse all 185 methods. Clicking a method performs one real
PTB request and reports the route and Telegram-shaped result. Send `/smoke` to
request the 183 non-terminal endpoints in sequence and receive a visible
summary. `logOut` and `close` are terminal Telegram lifecycle methods: click
either one from the catalog to exercise it individually, or set
`BOT_SMOKE_INCLUDE_LIFECYCLE=1` in a disposable environment to append both
requests after the other 183 calls. The latter can invalidate the token before
the bot can send a post-run reply, which is the expected Telegram behavior.

Probe IDs default to zero or invalid fixture values so mutation and payment
methods fail closed. For a disposable payment fixture, set
`BOT_PROBE_GIFT_ID` to the numeric string returned by `getAvailableGifts`
(for example `72`) along with the other probe IDs. Set
`BOT_PROBE_CHAT_ID`, `BOT_PROBE_USER_ID`, `BOT_PROBE_MESSAGE_ID`, and
`BOT_PROBE_FILE_ID` only in a disposable fixture environment when exercising
valid resource paths. Gift/payment probes can use
`BOT_PROBE_GIFT_ID`, `BOT_PROBE_OWNED_GIFT_ID`,
`BOT_PROBE_STAR_CHARGE_ID`, `BOT_PROBE_STAR_SUBSCRIPTION_ID`, and
`BOT_PROBE_GIFT_STAR_COUNT`; set the last value to `0` when exercising the
provider-backed no-cost gift upgrade/transfer paths.
