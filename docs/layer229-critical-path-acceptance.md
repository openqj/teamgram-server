# Layer 229 关键业务链路验收

这份矩阵把当前缺口拆成六类可重复验收：消息、频道、设置、Bot/Mini App、Premium、Business。矩阵数据在
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

# Premium fixture settlement plus backend restart (real refund/revoke stays manual)
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

### Bot/Mini App

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `bot.inline-webview-cross-account` | `docker/production/probe-db-only-bot-mini.ts` | 用户和 bot 两 session、inline 结果、prepared message、WebView、bot status | 更新 loop 被关闭、provider authority、重启 replay |
| `bot.webview-provider-boundary` | `docker/production/probe-webview-bot-business.ts` | WebView、requested peer、callback、Bot App、business control plane | 当前是本地 control-plane fixture，缺真实 provider 和 recipient fan-out |
| `bot.callback-payment-update-fanout` | `docker/production/probe-bot-message-gap.ts` | callback/webhook/custom request、shipping/precheckout、清理 pending rows | 外部 Bot API/payment provider、实时消费、重启 |
| `bot.web-ui-miniapp` | Web workspace 的 `tests/playwright-real/bot-miniapp-web.spec.ts` | 真实 WebSocket 登录、`/start`、主 Mini App iframe | provider 和重启证据不由 UI pass 推导 |
| `bot.backend-restart-replay` | 手工门槛 | 带 query/request ID 的 bot/WebView 操作跨 backend restart 且 exactly-once | 目前没有现成跨进程探针 |

### Premium

| 行 | 执行入口 | 覆盖 | 尚未关闭 |
| --- | --- | --- | --- |
| `premium.catalog-session` | `docker/production/probe-premium-readonly.ts` | Premium promo 和可选 payment form 构造器 | 真实目录/provider、权益变更、重启 |
| `premium.payment-idempotency-provider` | `docker/production/probe-premium-purchase.ts` | form、requested info、payment form、相同请求 replay | Apple/Google、退款/撤销、生产对账 |
| `premium.store-receipt-provider` | `docker/production/probe-premium-store-assignments.ts` | App Store/Play assignment 和 replay | 真实 receipt、退款/撤销、重启对账 |
| `premium.entitlement-restart-fixture` | `docker/production/probe-premium-entitlement-restart.ts` | A/B 两个真实 Layer 229 session 观察 Premium 权益，精确 replay，backend restart 后核对 provider/local receipt、entitlement、outbox 和 users 行 | 仅为本地签名 fixture 的结算与重启证据；真实 Apple/Google receipt、退款/revoke webhook、权益移除仍是手工门槛 |
| `premium.entitlement-refund-restart` | 手工门槛 | 两账号权益、重启后读回、退款撤销和 canonical update | 没有真实 store webhook/revoke 探针 |

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
