# Layer 229 关键业务链路验收

这份矩阵把当前缺口拆成七类可重复验收：消息、频道、设置、provider catalog、Bot/Mini App、Premium、Business。矩阵数据在
[`tools/critical_path_acceptance_matrix.json`](../tools/critical_path_acceptance_matrix.json)，执行器在
[`tools/critical_path_acceptance.py`](../tools/critical_path_acceptance.py)。执行器只运行已存在的探针并原样保存
stdout/stderr；它不会修改 `LAYER229_METHOD_LEDGER.csv`，也不会把跳过、组件测试或数据库读回写成生产通过。

## 验收门槛

| 门槛 | 必须证明 | 不能替代的证据 |
| --- | --- | --- |
| `cross_account` | A 写入后，B 使用独立 auth key/session 看到权限边界和对应对象 | 同一账号重新查询、直接查 PostgreSQL |
| `update_fanout` | B 的实时 Layer 229 updates 流收到目标 update，且 `pts/qts/seq/date` 连续 | B 重新调用 `getHistory` 或读取数据库 |
| `session_restart` | 销毁 A/B 客户端后用新 session 继续读写，并核对幂等/权限 | 同一客户端断线重连，或只看 HTTP 200 |
| `backend_restart` | 保存带 tag 的业务对象，重启 backend/session 服务，A/B 新 session 读回并继续操作 | 只重启 GramJS 客户端、只重启 Web UI |
| `provider_contract` | 签名、nonce、constructor、权限和失败关闭均通过真实 provider endpoint | 本地 `apifull_*` fixture、静态 JSON、单元测试 |
| `provider_restart_reconciliation` | 结算、重启、退款/撤销、两账号权益回收和对账记录完整 | 只返回 `PaymentResult`、只校验一次幂等 replay |

矩阵中的 `READY` 只表示本机脚本和必需环境存在；`PASS` 只表示该次命令退出码为零。两者都不提升台账的
`production_acceptance_status`，直到对应门槛的证据文件被人工复核并写入台账。

## 运行方式

在 `teamgram-server` 目录执行：

```sh
# 只读预检，不连接 backend，不改数据库
python3 tools/critical_path_acceptance.py --check

# 只看一个业务域
python3 tools/critical_path_acceptance.py --check --category channel

# 运行某一条可执行探针；明确 --run 才会执行有清理动作的探针
python3 tools/critical_path_acceptance.py \
  --run --id channel.cross-account-message-fanout \
  --output /tmp/teamgram-channel-acceptance.json

# 运行一个业务域的所有可执行行；MANUAL/BLOCKED 会保留在报告中并使退出码为 2
python3 tools/critical_path_acceptance.py \
  --run --category bot_miniapp \
  --output /tmp/teamgram-bot-miniapp-acceptance.json

# backend restart rows require an explicit destructive-run acknowledgement
RUN_RESTART_PROBE=1 python3 tools/critical_path_acceptance.py \
  --run --id channel.backend-restart-persistence \
  --output /tmp/teamgram-channel-backend-restart.json

# The same guarded runner accepts settings or business as its final argument.
RUN_RESTART_PROBE=1 tools/run-backend-restart-critical.sh settings
RUN_RESTART_PROBE=1 tools/run-backend-restart-critical.sh business

# Premium fixture settlement, refund/revoke, reconciliation plus backend restart
export ACCEPTANCE_COMPOSE_FILES="/path/to/docker-compose.yaml /path/to/backend-deploy.yaml"
export ACCEPTANCE_SERVICE=backend
RUN_BACKEND_RESTART=1 python3 tools/critical_path_acceptance.py \
  --run --id premium.entitlement-restart-fixture \
  --output /tmp/teamgram-premium-entitlement-restart.json
```

默认使用 `../telegram-tt-master` 作为 `TEAMGRAM_GRAMJS_DIR`。可以显式覆盖：

```sh
export TEAMGRAM_GRAMJS_DIR=/path/to/telegram-tt-master
```

`web-isolated` 探针默认连接 `127.0.0.1:31443`，生产形状探针默认连接 `127.0.0.1:11443`。需要真实 provider
或 auth key 的行会在缺少凭据时显示 `BLOCKED`，不会回退到假数据。探针执行后先保存 JSON 报告，再运行：

```sh
python3 tools/layer229_acceptance_report.py --strict
git diff --check
```

## 当前矩阵

完整字段和已知限制以 JSON 为准。下面列出每个域的执行入口和尚未关闭的门槛。

### 消息

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `messaging.cross-account-live-updates` | `docker/production/probe-cross-account-live-updates.ts` | 两个独立 auth key/session 保存 `updates.getState` 前后值，双向发送私聊，接收标准 `Updates` 容器，按 recipient state 校验消息 ID、`pts/pts_count/date`、历史回读并删除带 tag 消息 | 频道/Bot/Mini App、backend 重启、provider 和 Web UI 完整流程仍需各自验收 |
| `messaging.saved-dialog-filter-roundtrip` | `docker/production/probe-dialog-saved-layer229.ts` | 两个 Layer 229 session 创建/读取/排序 DialogFilters，并验证 Saved Message 的 `messages.Messages`、`messages.SavedDialogsSlice` 与 raw `messages.SavedDialogsByID` 构造器，最后恢复 saved-dialog 行和清理活动行 | native channel saved peers、backend 重启、独立设备 updates、外部 provider 与生产流量 |

### 频道

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `channel.cross-account-message-fanout` | `docker/web-isolated/probe-channel-message-lifecycle.ts` | 三账号消息创建、编辑、删除、成员/所有者可见性 | backend 重启、原生媒体、独立设备 updates 流 |
| `channel.cross-account-members-reconnect` | `docker/web-isolated/probe-channel-members.ts` | 管理员边界、成员移除、错误 access hash、新客户端读回 | 目前是客户端 session restart，不是 backend restart |
| `channel.invites-cross-account-fanout` | `docker/web-isolated/probe-channel-invites.ts` | 邀请导出、撤销、加入、批准/拒绝和清理 | 独立设备实时 fan-out |
| `channel.native-media-roundtrip` | `docker/production/probe-channel-media-production.ts` | 频道媒体、相册 grouped id、retry 幂等、历史 hydrate | 外部媒体/CDN、重启和生产 fan-out |
| `channel.backend-restart-persistence` | `tools/run-backend-restart-critical.sh` (`RESTART_PROBE_KIND=channel`) | 带 tag 的频道/成员/消息跨进程恢复，成员在重启后收到 canonical update，校验 `pts/date/seq` 连续和清理 | 生产 SMS 登录、外部媒体/CDN |

### 设置

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `settings.notifications-cross-account` | `docker/web-isolated/probe-notification-settings.ts` | 两账号通知隔离、异常列表、重置、设备注册清理 | 独立设备 updates、backend 重启 |
| `settings.session-and-password-reconnect` | `docker/web-isolated/probe-account-session-settings.ts` | authorization/web authorization、密码设置、第二客户端 | backend 重启、URL provider |
| `settings.global-privacy-roundtrip` | `docker/web-isolated/probe-global-privacy-settings.ts` | 全局隐私字段写读和恢复 | 独立设备 updates、backend 重启 |
| `settings.backend-restart-persistence` | `tools/run-backend-restart-critical.sh` (`RESTART_PROBE_KIND=settings`) | 通知设置保存原值、写入 tag marker，重启后读回、账号隔离并逐字段恢复 | provider-backed authorization、外部设备 push |

### Provider Catalog

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `provider.sponsored-collectible-catalog` | `docker/production/probe-sponsored-collectible-fixtures.sh` | 真实 DC2 session 读取 sponsored peers 和 owner-scoped collectible emoji，校验 collectible hash replay，并精确清理 PostgreSQL fixture rows | 广告/collectible 权威 provider、backend restart、生产流量 |
| `provider.stars-gifts-catalog` | `docker/production/probe-stars-gifts-catalog.ts` | 真实 DC2 session 读取 Stars top-up catalog、Stars balance、saved gift 和 active auction，校验 provider fingerprint 和全量清理 | Stars checkout/settlement、全局礼物库存/拍卖权威、backend restart、生产流量 |

### Bot/Mini App

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `bot.inline-webview-cross-account` | `docker/production/probe-db-only-bot-mini.ts` | 用户和 bot 两 session、inline 结果、prepared message、WebView、bot status | 更新 loop 被关闭、provider authority、重启 replay |
| `bot.webview-provider-boundary` | `docker/production/probe-webview-bot-business.ts` | WebView、requested peer、callback、Bot App、business control plane | 当前是本地 control-plane fixture，缺真实 provider 和 recipient fan-out |
| `bot.callback-payment-update-fanout` | `docker/production/probe-bot-message-gap.ts` | callback/webhook/custom request、paid-reaction privacy、shipping/precheckout、清理 pending rows | 外部 Bot API/payment provider、实时消费、重启 |
| `bot.web-ui-miniapp` | Web workspace 的 `tests/playwright-real/bot-miniapp-web.spec.ts` | 真实 WebSocket 登录、`/start`、主 Mini App iframe | provider 和重启证据不由 UI pass 推导 |
| `bot.backend-restart-replay` | `tools/run-backend-restart-bot-mini.sh` | 带 query/request ID 的 bot/WebView 操作跨 backend restart 且 exactly-once | 本轮使用本地 Bot/Mini App provider-shaped state；签名外部 Bot API/WebView authority、真实生产流量和支付回调仍需单独验收 |

### Premium

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `premium.catalog-session` | `docker/production/probe-premium-readonly.ts` | Premium promo 和可选 payment form 构造器 | 真实目录/provider、权益变更、重启 |
| `premium.payment-idempotency-provider` | `docker/production/probe-premium-purchase.ts` | form、requested info、payment form、相同请求 replay | Apple/Google 真实 receipt；退款/撤销和生产对账由恢复门槛覆盖 |
| `premium.gift-collections-session` | `docker/production/probe-gift-collections.ts` | 真实 Layer 229 session 创建、读取/hash NotModified、更新、排序、删除 Star Gift collections；mutation exact replay 和 tagged PostgreSQL cleanup | 签名 PostgreSQL fixture 之外的 gift inventory/ownership provider、backend restart、跨设备 fan-out、生产流量 |
| `premium.store-receipt-provider` | `docker/production/probe-premium-store-assignments.ts` | App Store/Play assignment 和 replay | 真实 receipt、退款/撤销、重启对账 |
| `premium.entitlement-restart-fixture` | `docker/production/probe-premium-entitlement-restart.ts` | A/B 两个真实 Layer 229 session 观察 Premium 权益，精确 replay，backend restart 后核对 provider/local receipt、entitlement、outbox、reconciliation、webhook reversal、grant tombstone 和 users 行 | 仅为本地签名 fixture；真实 Apple/Google receipt 和真实 store webhook 仍是手工门槛 |
| `premium.entitlement-refund-restart` | 手工门槛 | 两账号权益、重启后读回、真实 provider 退款撤销和 canonical update | 需要真实 Apple/Google receipt、store webhook 凭证及生产 provider 对账证据 |

### Business

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `business-profile-cross-session` | `docker/production/probe-business-profile.ts` | work hours、location、greeting、away、intro、主 profile tab | userFull hydration、recipient fan-out、重启 |
| `business-provider-operations` | `docker/production/probe-business-provider-operations.ts` | Fragment/callback/live provider 边界和 typed result | 当前可由本地 fixture rows 驱动，缺生产 provider/轮换/故障恢复 |
| `business-webview-cross-account` | `docker/web-isolated/verify-business-flows.sh` | 14 个真实 fixture 账号的业务链接、WebView、消息和清理 | 签名 provider、backend restart、recipient updates |
| `business.backend-restart-and-hydration` | `tools/run-backend-restart-critical.sh` (`RESTART_PROBE_KIND=business`) | BusinessChatLink/resolver、`users.getFullUser`、recipient update 跨重启并清理 | 签名外部 Business provider authority |

## Restart runner

三条 backend restart 行由 `tools/run-backend-restart-critical.sh` 编排。
runner 默认使用现有 `backend` Compose project 和 layered production fixture，要求显式设置
`RUN_RESTART_PROBE=1`；也可以通过 `ACCEPTANCE_COMPOSE_FILES`、`ACCEPTANCE_SERVICE`、
`TG_DC_HOST` 和 `TG_DC_PORT` 覆盖目标。每次执行会生成带 tag 的 state/report JSON，完成后自动恢复或删除
探针对象。`probe-backend-restart.ts` 的 `channel`、`settings`、`business` 三种 kind 都按
`before -> restart -> after -> cleanup` 顺序运行。

Premium fixture 重启探针在执行 Compose restart 前会从当前 backend 容器补齐
`TEAMGRAM_CODE_PROVIDER_KEY`、`TEAMGRAM_CODE_PROVIDER_SECRET` 和
`TEAMGRAM_CODE_CHALLENGE_SECRET`（显式环境变量优先），避免运行时使用的旧变量名导致 Compose 插值失败。

### Restart readiness race 记录

首次 Business barrier 中，Compose 已报告 `backend` 为 `healthy`，但 after/cleanup 的第一次
GramJS 连接仍收到 WebSocket `ErrorEvent`。根因是容器 healthcheck 早于 MTProto listener 完全接受连接，
不是 Business 数据丢失。修复为 probe actor 在连接和 `getMe` 阶段最多重试四次（间隔 750 ms），并将
state 文件与最终 report 分离；cleanup 在 state 缺失时按 tag 查找 BusinessChatLink。修复后的真实
证据均为 `PASS`：

* `/tmp/teamgram-backend-restart-channel-backendrestart-1791916215-3371.json`
* `/tmp/teamgram-backend-restart-settings-backendrestart-1791916684-6550.json`
* `/tmp/teamgram-backend-restart-business-backendrestart-1791916339-3905.json`

上述报告的 before、Compose restart barrier、after 和 cleanup 步骤均为零退出码；Business 报告还包含
`users.UserFull` 和 recipient update 的 `id/pts/date`。

### 持久会话重连记录

一次直接重启 `backend-backend-1` 的持久会话探针复现了 readiness 窗口问题：WebSocket 已断开后，
`MTProtoSender._reconnect()` 的单次连接尝试在 backend 尚未重新接受 MTProto 连接时失败，随后
`_userConnected=false` 使心跳不再发起重连。修复位于 `telegram-tt-master/src/lib/gramjs/network/MTProtoSender.ts`
和 `telegram-tt-master/src/lib/gramjs/network/connection/Connection.ts`：重连期间创建新的传输对象并按退避持续
尝试，显式断开仍可终止循环，失败对象会关闭底层 socket。真实双账号证据：

* `/tmp/teamgram-connection-stability-backend-restart-final.json`：backend 进程重启后 36/36 RPC 成功，A/B
  `updates.State` 的 `pts/qts/seq/date` 未回退，双向消息 fan-out 和带 tag 消息清理通过。
* `/tmp/teamgram-connection-stability-11443.json`、`/tmp/teamgram-connection-stability-31443.json`：
  两个 TCP 网关各自完成主动断链、自动恢复和双向消息回归。

### 2026-10-14 重跑记录

本轮使用独立 fixture session 完成的关键报告：

* `/tmp/teamgram-agent-restart-channel-final.json`
* `/tmp/teamgram-agent-restart-settings.json`
* `/tmp/teamgram-agent-restart-business.json`
* `/tmp/teamgram-agent-restart-bot-mini-final.json`
* `/tmp/teamgram-agent-channel-cross-account-members-reconnect.json`
* `/tmp/teamgram-agent-invites-cross-account-fanout.json`
* `/tmp/teamgram-agent-channel-settings-global-privacy-roundtrip.json`

三条重启报告的 `before`、Compose restart barrier、`after` 和 `cleanup` 均为 `PASS`，其中 channel
报告验证了 A/B 新 session 的历史读回和成员 update fan-out，business 报告验证了 `users.UserFull`
和 recipient update。channel 重启所用的既有 acceptance image 在删除 probe 频道后留下了一条
editable username 行；该行已按精确 probe tag 清理。源码的 `DeleteChannel` 事务现在同步删除
`peer_type=PEER_CHANNEL` 的 editable username，并由 `TestChannelsDeleteChannelOwnerTransactionRoundTrip`
覆盖；由于 Docker 构建网络无法解析 `ports.ubuntu.com`/`proxy.golang.org`，包含该修复的新 acceptance image
尚未完成重启部署验证。

### 2026-10-18 Stars/provider 重跑记录

本轮使用 user 7 的活动 auth key，在 DC2 TCPObfuscated -> gateway -> session -> APIFull 上完成四组
payment/provider 探针；认证 key 只注入子进程环境，报告不包含 secret：

* `/tmp/teamgram-agent-stars-topup.json`：StarsTopupOption、`payments.PaymentFormStars`、精确
  `sendStarsForm` replay 和一次 provider settlement；余额在清理前由 0 增至 101，tagged rows 清理后为零。
* `/tmp/teamgram-agent-stars-refund.json`：Stars settlement/replay、退款/replay 均返回 `Updates`，一条
  refund row 和一条 provider refund operation，余额恢复为 0；清理后 request/provider/refund/offer rows 为零。
* `/tmp/teamgram-agent-star-referral.json`：五个 Star referral 方法均返回 typed `SuggestedStarRefBots`/
  `ConnectedStarRefBots`，连接设置 `revoked=true` 后清理 program/connection rows。
* `/tmp/teamgram-agent-star-gift-payment-operations.json`：15 个 giveaway/upgrade/withdrawal/resale/
  auction/offer/craft 方法均返回 typed Layer 229 结果，15/15 provider rows 清理，`remaining=0`。
* `/tmp/teamgram-agent-gift-collections.json`：真实 user10 session 完成五个 Star Gift collection 方法；列表
  hash `NotModified`、update/reorder exact replay 和 typed `StarGiftCollection`/`Boolean` 通过，cleanup 后
  collections/items/gifts 均为 0。该报告只证明签名 PostgreSQL fixture 边界。
* `/tmp/teamgram-account-update-status-1792269671613.json`：fixture user11 的两个永久 auth-key session 在
  DC2 TCPObfuscated 上完成 online/offline 写入，B 收到 canonical `UpdateUserStatus`，A 未收到自身 update，
  PostgreSQL presence 恢复原值。联系人隐私收件人解析、backend restart 和生产流量仍未关闭。

这些报告是本地签名 payment/business fixture 与 PostgreSQL 控制面的 `PARTIAL_ISOLATED_SESSION_E2E`
证据；Apple/Google receipt、真实 Stars/gift/referral provider、生产退款 authority、backend restart
reconciliation 和生产流量仍由对应手工门槛覆盖。

## 手工重启门槛

为了避免把“客户端重连”误记为“服务重启”，手工行要求调用方显式提供 Compose 文件和服务名：

```sh
export ACCEPTANCE_COMPOSE_FILE=/path/to/docker-compose.yml
export ACCEPTANCE_SERVICE=backend
docker compose -f "$ACCEPTANCE_COMPOSE_FILE" restart "$ACCEPTANCE_SERVICE"
```

重启前必须保存对象 tag、账号 A/B、数据库关键行 hash 和最后一个 update state；重启后必须用新 auth
session 读取同一 tag，验证 update recipient、幂等重放和 `pts/qts/seq/date`，最后恢复原始状态。没有这些
字段的日志只能作为 `PARTIAL_*` 证据。

## Provider 和生产证据

`provider-fixtures.acceptance.yaml`、payment fixture、business fixture 和 identity fixture 只证明签名协议和
Layer 229 构造器边界。它们不能证明 Apple/Google、Bot API、Mini App/WebView、广告、CDN 或真实业务 provider
可用。生产 provider 行应同时附上 endpoint、请求 nonce、签名验证、超时/故障关闭、重试幂等和密钥轮换记录；
敏感 token 只能放在运行环境或密钥管理系统，不要写进 JSON 报告或 ledger。

## 台账更新规则

探针通过后，人工从报告中提取可复核字段，更新对应 `LAYER229_METHOD_LEDGER.csv` 的
`production_acceptance_evidence`，再执行 `layer229_acceptance_report.py --strict`。只有同时满足对应门槛且有
真实 session/provider/restart/fan-out 证据，才可以从 `PARTIAL_*` 提升状态；运行器本身不会自动提升状态。

## 2026-10-18 Agent rerun evidence

The three parallel Layer 229 acceptance chains produced the following evidence:

* Messaging/channel/call: `/tmp/teamgram-agent-messaging-cross-account-live-updates.json`, `/tmp/teamgram-agent-messaging-multi-media-layer229.json`, `/tmp/teamgram-agent-messaging-chat-reactions-layer229.json`, `/tmp/teamgram-agent-calls-group-control-layer229.json`, `/tmp/teamgram-agent-channel-native-media-roundtrip-layer229.json`, and `/tmp/teamgram-agent-channel-access-hash-boundary-layer229.json`. These reports use explicit `InvokeWithLayer(229)` over DC2 TCPObfuscated and cover cross-account Updates, `pts` continuity, random-id replay, media/grouped IDs, reactions, group-call control, channel difference recovery, canonical permission errors, and cleanup. The earlier reports without a `layer` field are not counted for this rerun.
* Account/settings: `/tmp/teamgram-agent-settings-backend-restart.json` passed the guarded production Compose `before -> restart -> healthy barrier -> after -> cleanup` flow. Fresh A/B sessions read back the settings marker after restart without cross-account leakage; content settings, global privacy, contact export token, and TTL probes restored their original PostgreSQL state. The isolated gateway at `127.0.0.1:31443` was not running, so its failed web-isolated probes remain failures rather than production evidence.
* Payments/business/Bot: `/tmp/teamgram-agent-stars-refund-provider-1792267506.json`, `/tmp/teamgram-agent-star-gift-payment-operations-1792267563.json`, `/tmp/teamgram-agent-business-provider-operations-1792267747.json`, `/tmp/teamgram-agent-bot-webview-provider-boundary-1792267808.json`, and `/tmp/teamgram-agent-bot-callback-payment-update-fanout-1792267866.json` passed their typed-result, idempotency, callback, and cleanup assertions. `/tmp/teamgram-agent-premium-entitlement-fixture-no-restart-fixed-1792268827.json` passed payment replay, entitlement observation, signed refund webhook, reversal, and reconciliation; it explicitly skipped a second restart and retained durable reconciliation rows.

These are isolated session/provider and one guarded local restart gates. They do not close external Apple/Google receipt authority, real store refunds, external Bot/WebView/Stars/CDN/TURN/Live providers, production traffic, or full cross-device fan-out. Run `python3 tools/layer229_acceptance_report.py --strict --json` after any ledger evidence edit and preserve the resulting status counts.

## 2026-10-18 Final Three-Chain Rerun

* Messaging/channel/call: `/tmp/teamgram-agent-calls-group-control-layer229-rerun.json` and `/tmp/teamgram-agent-channel-native-media-roundtrip-layer229-rerun-2.json` both passed with explicit Layer 229 over DC2 TCPObfuscated. The channel retry after the initial listener-readiness race passed grouped media, `updates.ChannelDifference`, exact random-id replay and zero tagged deletion rows.
* Account/auth: `/tmp/teamgram-agent-account-auth-provider-layer229-final-1792272418.json` and `/tmp/teamgram-agent-account-passkey-layer229-final-1792272446.json` passed with forced `InvokeWithLayer(229)`, covering code-provider send/sign-in/cancel and passkey registration/list/login/logout/delete. Real SMS, hardware RP credentials and authsession restart remain separate gates.
* Payment: `/tmp/teamgram-agent-stars-refund-provider-20261018.json` passed Stars payment/refund replay, cleanup and payment-fixture restart replay. Backend restart, external store/Stars authority, webhook reconciliation and production traffic remain open.

The guarded backend restart barrier also passed for all three supported restart probes: `/tmp/teamgram-agent-channel-backend-restart-final.json`, `/tmp/teamgram-agent-settings-backend-restart-final.json`, and `/tmp/teamgram-agent-business-backend-restart-final.json`. Each report contains a matching backend image check, before marker, real Compose restart, healthy barrier, fresh-session after assertion, and cleanup. This closes the local channel/settings/business restart gate; it does not close external provider, cross-instance failover, store authority or production-traffic gates.
