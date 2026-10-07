# Layer 229 API 功能验收

## 台账唯一来源

`LAYER229_METHOD_LEDGER.csv` 是本项目 Layer 229 方法台账的唯一权威来源。方法数量、`audit_status`、`production_acceptance_status` 及对应证据均以该文件为准；历史重复文件 `PARTIAL_COMPONENT_ONLY` 已退役，后续不再读取或更新。

## 2026-10-07 纵向业务链后端加固（未验收）

- 原生频道文本写入新增 `random_id` 请求映射和 migration `migrate-20261007-channel-message-idempotency.sql`；同一键重试会返回原消息，键复用但正文/回复不同则拒绝。频道编辑和 `updates.getChannelDifference` 现在验证 `InputPeer`/`InputChannel` access hash 与成员关系。APIFull 单条非计划频道媒体和非计划频道相册现通过 Media provider 写入内容 JSON canonical 行及差分事件；相册以单事务写入所有项，逐项 `random_id`、完整相册随机 ID 列表和原始输入指纹共同做重试冲突校验，历史与差分恢复 `grouped_id`。`migrate-20261008-channel-message-media.sql` 和 `migrate-20261007-channel-message-idempotency.sql` 尚未应用，也未做真实频道 session 读写验收。计划媒体仍返回 `METHOD_NOT_IMPL`。
- 支付入口现在只允许 `inputInvoiceSlug` + `inputStorePaymentPremiumSubscription` 自订阅；restore、upgrade、礼品和其他商品在打开结算前失败关闭。APIFull 将本人受益人和所需商品发给 provider，并校验签名 form/settlement 的用户、form、请求指纹、商品、月数和交易号。已结算账单会保留完整签名响应；User 服务用 provider+交易号幂等授予 Premium，并在同一事务内写授予记录和更新到期时间。命名锁继续串行同一请求，provider endpoint 限制为 HTTPS 或 loopback HTTP。外部 provider 未配置，provider 对请求商品约束的执行、退款、对账和真实结算仍未验收。
- 登录/改号/邮箱 challenge 使用独立 `Code.ChallengeSecret` 做 HMAC；生产配置要求至少 32 字节。共享 SMS/Email provider 仅允许 HTTPS 或 loopback HTTP；旧 `me` GET provider 只允许 loopback HTTP。challenge 与电话码清理使用独立短时 context。仓库示例配置的 challenge key 和 SMS provider 仍为空，所以该链不能据此视为已配置或生产可用。
- Bot registry 的创建者/父子关系写链已有实现；creator-index 与 manager-hierarchy 迁移现在按 `information_schema` 补缺列/索引，可用于基础 schema 和已升级数据库，但仍未应用。`bot_can_manage_bots` 继续默认关闭，只允许运维通过受控数据库操作授予，客户端没有授予入口。语音与群通话保留状态控制/信令记录；TURN/media relay、录制和真实音视频转发仍依赖未配置的外部基础设施。
- 本轮 `go build ./app/service/biz/user/... ./app/bff/apifull/...` 通过；没有运行测试、应用数据库迁移或调用生产 provider/探针。新增的 `migrate-20261008-user-premium-payment-grant.sql` 尚未应用，production acceptance 状态不提升。

## 2026-10-07 Premium 自订阅结算与权益发放

- `payments.getPaymentForm` 与 `payments.validateRequestedInfo` 只接收本人 Premium 订阅用途。provider 需要返回带 HMAC 的商品与月数元数据；form 必须是普通 `payments.paymentForm`，非本人商品元数据或其他 invoice 均拒绝。
- `payments.sendPaymentForm` 在扣款调用前检查 invoice 类型和 form ID。发送结算时要求 provider 处理 `premium_subscription` 且受益人是当前用户；结算回包必须签名绑定同一 form、用户、invoice 指纹、产品、月数和交易号。完整签名回包进入 canonical APIFull 账本。
- User 服务以 provider+transaction 的 SHA-256 键防止一笔交易重复延长 Premium；授予记录和 `users.premium_expire_date` 同事务提交。APIFull 在已结算重试时从账本恢复收据并重试权益授予，不会再次请求扣款。手工 Premium 更新也修复了续期后又被覆盖的问题。
- `go build ./app/service/biz/user/... ./app/bff/apifull/...` 通过。测试、迁移应用和生产验收未执行；当前没有已配置 provider，因此该链仍不是生产可用结算。

## 2026-10-07 Bot registry 创建与 token 写链

- `bots.createBot` 的标准路径已接入 User 服务权威写入：同一 MySQL 事务校验创建者、写入 bot 用户、唯一用户名索引和 `bots` registry 行，并生成加密随机 token。用户名检查仍是 advisory，唯一索引与事务决定最终占用结果。
- `bots.exportBotToken` 由 User 服务校验目标是 bot、access hash 匹配且调用者是记录的创建者；读取和轮换在锁定 registry 行后执行。Bot 用户资料不再携带 token，token 请求和响应不写入调试日志；两个内部 token 用户查询只把查无记录映射为 `TOKEN_INVALID`，数据库错误继续向上返回。
- `bots.createBot` 的 deeplink 分支现通过 User registry 接收父 Bot ID 与 access hash，并在同一创建事务内校验父 Bot 的 `bot_can_manage_bots`。父 Bot 关系与人类创建者分开保存；子 Bot token 仍只由 `creator_user_id` 导出。能力默认关闭，仅允许运维通过受控数据库操作授予，客户端没有授予入口。`go build ./app/service/biz/user/... ./app/bff/apifull/...` 通过；本轮未运行测试、应用迁移或连接数据库/真实会话/生产服务，因此迁移、创建回滚、owner 隔离和 token 轮换仍未验收。

## 2026-10-07 Bot direct-start 消息写链

- `messages.startBot` 校验 Bot 注册资料、`InputUser`/`InputPeer` access hash 和 64 字节 ASCII deep-link 参数，再把 `/start` 命令经 canonical Msg 服务投递到 Bot 私聊；消息使用请求的 `random_id` 做幂等处理。
- 群组/频道启动继续失败关闭，因为当前没有验证 Bot 成员权限并向群组投递启动命令的 provider。APIFull 编译通过；本轮未运行测试、数据库或会话验收，所以台账状态仍为 `BLOCKED_BY_IMPLEMENTATION_GAP`。

## 2026-10-07 r20 频道删除事件清理生产验收

- `backend-backend-1` 已更新到 `teamgram-server-latest:20261007-r20-prod`。生产授权用户 `136907714` 经 DC2 WebSocket -> gateway -> session -> APIFull，在真实 MySQL 创建临时频道并发送一条消息；消息行与一个 `apifull_channel_event` 事件均成功持久化。
- 同一会话的 `updates.getChannelDifference` 返回 `updates.ChannelDifference`，其中包含这条消息。随后 `channels.deleteChannel` 返回 `Updates`；频道、成员、消息、序列、事件、隐藏状态、内容已读、阅读状态和管理日志均读回 0 行。
- 数据库仍有 90 张表，删除前已存在的 103 条孤儿频道事件保持不变。没有执行手工 schema migration；只替换 backend，MySQL、Redis、etcd、Kafka 的容器 ID 与重启计数未变。
- 这验收了一个真实 owner 的创建、消息、差分读取和频道删除路径；多会话推送、重启回放以及其余频道事件类型仍未覆盖。

## 2026-10-07 r20 频道慢速模式生产回环

- 生产用户 `136907714` 经 DC2 WebSocket -> gateway -> session -> APIFull，在新建临时频道上调用 `channels.toggleSlowMode(seconds=17)`，返回 `Updates`。`channels.getFullChannel` 和 MySQL `apifull_channel.slowmode_seconds` 均读回 `17`。
- 同一会话将慢速模式恢复为 `0`，API 与 MySQL 均读回 `0`，随后由频道 owner 删除临时频道并确认频道行消失。探针用 Layer 229 构造器 `#edd49ef0` 原始编码，因为当前 GramJS 运行时没有生成该客户端构造器。
- 同一个独占临时频道还真实往返了 `channels.toggleParticipantsHidden` 与 `channels.togglePreHistoryHidden`：各自返回 `Updates`，`getFullChannel` 和对应 MySQL 布尔列均读回 `true`，恢复调用后再次读回 `false`。
- 这覆盖了真实 owner、协议传输、可逆配置持久化和清理；非 owner 权限已由隔离库测试覆盖，原生频道 provider 路径仍未验收。

## 2026-10-07 Stars 只读账本分支

- 生产用户 `136907714` 经 DC2 WebSocket 查询 `payments.getStarsTransactions`，返回 `payments.StarsStatus(balance=0, history=0)`。生产 `apifull_stars` 与 `apifull_star_tx` 中该用户均无行；全库分别有 4 个余额行和 51 条交易，调用前后目标数据未改变。
- `payments.getStarsStatus` 对同一未配置余额的用户返回 `METHOD_NOT_IMPL`，未把它计为生产通过。只有零余额/空流水分支得到真实生产证据；非空交易、入账、结算与 entitlement 仍未覆盖。

## 2026-10-07 默认历史 TTL 生产可逆验收

- 新增 `docker/production/probe-default-history-ttl-roundtrip.ts`，从生产 MySQL 中选择用户 `136907714` 的有效永久授权并只在探针进程内使用，密钥不输出。真实客户端经 DC2 WebSocket -> gateway -> session -> privacy settings BFF -> biz user -> 生产 MySQL 完成 `messages.getDefaultHistoryTTL`、`messages.setDefaultHistoryTTL` 与 MySQL 对照读取。
- 探针先要求 API 值与已有 `default_history_ttl` 行一致，再临时设为一天、验证 `BoolTrue`、API 和 MySQL 均读回一天，随后恢复原值并再次验证。不存在预置行或发现并发改值时拒绝执行/覆盖。
- 这证明该用户单会话下默认 TTL 的协议、BFF、User 服务和生产持久化回环；其它用户、跨会话推送及进程重启后的恢复仍未验收。

## 2026-10-07 频道消息统计生产只读验收

- 新增 `docker/production/probe-message-stats-readonly.ts`，使用生产用户 `136907714` 的有效授权，自动选择该用户拥有且生产库中确有消息的频道，调用 `stats.getMessageStats`。
- 请求经 DC2 WebSocket -> gateway -> session -> BFF/APIFull -> 生产 MySQL 返回 `stats.MessageStats` 和 `StatsGraph`。探针将图表中的阅读数与同一频道/消息在 `apifull_channel_read_state` 的已持久化游标计数核对一致；本次只读、写入数为 0。
- 这证明真实频道授权、消息存在性、统计图表编码及阅读游标查询。反应统计和完整 analytics provider 仍未实现，非 owner/admin 权限分支也未在生产验证。

## 2026-10-07 `messages.getMessageEditData` 生产只读验收

- 生产用户 `136907714` 经 DC2 WebSocket `127.0.0.1:11443` -> gateway -> session -> messages BFF -> APIFull/MySQL 调用 `messages.getMessageEditData#fda68d36`，peer 为 `InputPeerChannel(channel_id=1790115796396772258)`，`id=7`。授权 key 从生产 MySQL 查询后只保留在探针进程内，没有输出或落盘；本轮没有部署或重启容器。
- GramJS 类型声明包含该请求，但运行时未导出构造器；探针在内存中按 TL schema 编码 `#fda68d36` 后发送。`messages.getHistory` 对同一 peer 返回 `messages.Messages` 的 7 条消息，其中 `id=7` 的 sender 与消息文本 SHA-256 均匹配生产 `apifull_channel_message` 行；目标请求返回 `messages.MessageEditData(caption=false)`。
- 请求前后生产消息行相同，写入数为 0。样本频道 creator 在 `apifull_channel_member` 中没有单独 member 行；当前 `LoadChannelMember` 会将 creator 解析为隐式成员，因此与 r20 的成功结果一致。此证据只覆盖 APIFull 文本消息，不覆盖原生频道消息或媒体 caption，状态保持部分验收。

## 2026-10-07 r25 频道可空内容读取修复与生产复测

- r24 的 `messages.getMessageEditData` 和 `messages.getHistory` 在旧频道行的 `content_json=NULL` 上返回 500。根因是 APIFull 频道消息读取路径把可空列直接扫描到 `string`；本次将消息、置顶/编辑和差分读取统一改为可空字符串，NULL 按无附加内容处理。
- r25 (`teamgram-server-latest:20261007-r25-prod`) 仅替换 `backend-backend-1`，容器 `running`、`restart=0`、`oom=false`。原始 TL `messages.getMessageEditData#fda68d36` 经 DC2 WebSocket -> gateway -> session -> messages BFF -> APIFull/MySQL 返回 `messages.MessageEditData(caption=false)`；同频道 `messages.getHistory` 返回 7 条，sender/date/text SHA-256 与 MySQL 逐条一致。请求前后目标行不变，业务写入为 0。
- r25 启动后的 MySQL binlog 位置保持 `binlog.000004:6866222`，没有 `CREATE/ALTER/DROP`；mysql、redis、etcd 重启计数均为 0，Kafka 保持此前历史计数 2。Passkey/APIFull 只读启动保护和 dialog filter schema 存在性检查均随镜像生效。
- 新增 `teamgramd/deploy/sql/migrate-20261007-apifull-schema-complete.sql`，显式覆盖 APIFull runtime 依赖的五张审计/Stars/支付表及频道配置列。当前生产库已有这些表，但实际挂载 SQL 目录尚未同步该文件；在新库或切换只读模式前必须先执行完整迁移并做 schema preflight。完整 Layer 229 仍不是 813/813，原生频道媒体、支付 provider、短信/邮件、Passkey 真实认证器等外部依赖继续按台账阻塞。

## 2026-10-07 r24 `messages.getMessageEditData` 生产回归复测

- `backend-backend-1` 当前为 `teamgram-server-latest:20261007-r24-prod`，容器 `running`、`restart=0`。使用同一生产用户、频道 `1790115796396772258` 和数据库确认的 TL `id=7`，按原始 `messages.getMessageEditData#fda68d36` / `InputPeerChannel` 请求只读调用；GramJS 构造器仍在运行时缺失，TL 帧仅在内存中编码。
- r24 返回类型不是 `messages.MessageEditData`，而是 `RPCError 500: sql: Scan error on column index 9, name "content_json": converting NULL to string is unsupported (caused by messages.GetMessageEditData)`。同一部署的 `messages.getHistory` 也在该生产文本行的 `content_json=NULL` 上返回相同扫描错误。
- 请求前后 `apifull_channel_message` 行（`channel_id=1790115796396772258`, `message_id=7`, sender `136907714`, 文本 SHA-256 `85e1cd...e1e9d52`）完全一致，写入数为 0。r20 的成功证据仍保留，但当前 r24 复测失败；CSV 继续保持 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`，不应提升为通过。

## 2026-10-07 群通话设置、录制控制和参与者状态生产回环

- 新增 `docker/production/probe-group-call-settings.ts`，使用用户 `136907713` 的有效永久授权创建临时群通话，经 DC2 WebSocket -> gateway -> session -> BFF/APIFull -> 生产 MySQL 完成控制面回环。
- `phone.toggleGroupCallSettings` 返回 `Updates`；`phone.getGroupCall` 读回 `join_muted=true`、`messages_enabled=false`、付费消息阈值 `10`，并与 `apifull_group_call_settings` 行一致。
- `phone.toggleGroupCallRecord` 启动/停止录制控制，标题、视频和 portrait 状态由 `phone.getGroupCall`/MySQL 读回；`phone.editGroupCallParticipant` 在真实加入后更新 mute、视频停止和举手状态，并与 `apifull_group_call_participant` 一致。通话销毁后主表、设置表和参与者表均为 0 行。
- 这验证的是授权、协议编码和持久化控制状态；没有录制器、媒体片段存储、实时媒体 relay 或跨客户端更新消费者，因此不能把录制文件或音视频功能视为已完成。

## 2026-10-07 r16 群在线数生产只读复核

- `messages.getOnlines#6e2be050` 使用生产用户 `136907714` 查询 basic group `120031`，经 `DC2 WebSocket -> gateway -> session -> dialogs BFF -> biz chat/user -> 生产 MySQL` 返回 `messages.ChatOnlines`。
- 返回 `onlines=0`；群成员来自 `chat_participants`，在线窗口读取 User last-seen 数据，探针写入数为 0。
- 这证明了群 peer 校验、成员/presence 查询和协议编码；瞬时在线变化、跨会话更新和重启持久性仍未验收。

## 2026-10-07 r16 PeerSettings 生产只读复核

- `messages.getPeerSettings#efd9a6a2` 使用用户 `136907714` 查询真实用户 `136907713`，经 `DC2 WebSocket -> gateway -> session -> dialogs BFF -> biz user -> 生产 MySQL` 返回 `messages.PeerSettings`。
- 返回包含 `settings=PeerSettings`、一个对端 `User`、零个 `Chat`；生产 `user_peer_settings` 没有预存行，探针写入数为 0，证明默认设置和实体 hydration 的真实编码路径。
- 群组/频道成员权限分支、跨会话更新和重启持久性仍未验收。

## 2026-10-07 r16 用户实体生产只读复核

- `users.getUsers#d91a548` 使用生产用户 `136907714` 的有效授权，经 `DC2 WebSocket -> gateway -> session -> users BFF -> biz user -> 生产 MySQL` 读取 `InputUserSelf` 与真实用户 `136907713`。
- 返回向量包含两个类型化 `User`，ID 为 `136907714`、`136907713`；access hash 分别为 `574680665283270059`、`1398430508110637198`，与生产 `users` 表一致。探针写入数为 0。
- 同一会话随后调用 `users.getFullUser(InputUserSelf)`，返回 `users.UserFull`，其中 `fullUser.id=136907714`；这覆盖了 User/Chat/Dialog 依赖的真实读取和包装编码。更广泛隐私语义、频道规则、跨会话更新和重启持久性仍未验收。

## 2026-10-07 r16 广播统计生产只读复核

- 使用生产用户 `136907714` 对其 canonical 广播频道 `1790103377888636343` 调用 `stats.getBroadcastStats`，经 `DC2 WebSocket -> gateway -> session -> BFF/APIFull -> 生产 MySQL` 返回 `stats.BroadcastStats`。
- 返回统计包括 `followers.current=1`、`viewsPerPost.current=0`，近期帖子和消息交互向量均为空，完整 TL 编码长度为 472 字节；探针写入数为 0。
- 这证明了 owner 权限、频道 access hash、成员计数/阅读数读取和统计结果编码。外部 analytics provider、非空互动数据、其它统计维度仍未验收。

## 2026-10-07 r16 礼物目录生产只读复核

- `payments.getStarGifts#c4563590` 已通过有效生产永久授权完成 `DC2 WebSocket -> gateway -> session -> BFF/APIFull -> 生产 MySQL` 只读调用。用户 `136907714` 返回 `payments.StarGifts`，包含 2 个 `StarGift`（ID `1`、`5`），每项的必填 sticker 联合类型为 `DocumentEmpty`，完整 TL 编码长度为 120 字节。
- 同一有效会话随后以 slug `prod-gift` 调用 `payments.getUniqueStarGift`，返回 `payments.UniqueStarGift`，其中 `gift.id=1` 且 sticker 仍为 `DocumentEmpty`；目录和单项查询均未写入生产状态。
- 生产库 `apifull_gift` 当前有 35 行、2 个唯一 slug（`db-gift`、`prod-gift`）；服务目录按 slug 去重后返回两项。探针只读，写入计数为 0，没有修改礼物所有权或 Stars 余额。
- 这证明了本地权威目录读取和必填媒体 union 的协议编码；外部库存同步、真实 Stars 结算、非空 sticker 媒体、转售/提现等礼物业务仍未验收。

## 2026-10-07 r16 `channels.getChannelRecommendations` 生产 session 复核

- 路由链路已核对：`app/interface/session/etc/session.yaml` 将 `/mtproto.RPCChannelRecommendations` 映射到 `bff.bff`，BFF 注册 `RPCChannelRecommendations`，APIFull 服务再调用 `ChannelsGetChannelRecommendations`。构造器为 `channels.getChannelRecommendations#25a71742`，可选参数为 `InputChannel`，返回 `messages.Chats`。
- 使用生产用户 `136907714` 的现有授权，经 DC2 WebSocket -> gateway -> session -> BFF -> APIFull -> 生产 MySQL 只读调用。省略 `channel` 时返回 `messages.Chats`，其中 6 个频道与 `apifull_channel` 中该用户的 6 条记录一致；传入同一生产行的有效 `InputChannel` 时返回 5 项，并排除了该频道。两次响应均完成类型化编码。
- 当前实现读取用户创建的规范频道列表，并按可选频道 ID 排除；这证明了路由、canonical store 读取、响应编码和排除逻辑，但不等同于相似度排序或外部推荐 provider。该方法应保持 `PARTIAL_ISOLATED_SESSION_E2E`，推荐业务语义仍是功能缺口。
- 探针在响应打印后退出时出现客户端 `Not connected` / WebSocket `1005` 关闭噪声；两次 RPC 已先成功返回，不把退出阶段噪声记录为服务端错误。

## 2026-10-07 r16 保存消息非空 roundtrip 修复与生产复核

- `messages.getSavedHistory` 的服务层现在把 `PEER_SELF` 和指向当前用户的 `PEER_USER` 规范化为保存消息表实际使用的 `PEER_USER` 键。此前 `messages.sendMessage(InputPeerSelf)` 写入 `saved_peer_type=2`，而 `getSavedHistory(InputPeerSelf)` 用 `saved_peer_type=1` 精确查询，导致真实消息已写入但读回为空。
- 新增生产探针 `docker/production/probe-saved-dialog-roundtrip.ts` 的 peer 选择，使用生产 MySQL 中登记的用户 `136907713` 永久授权，经 `DC2 WebSocket -> gateway -> session -> r16 biz/BFF -> msg -> MySQL` 完成两次临时消息 `send -> getSavedHistory -> getSavedDialogs -> getSavedDialogsByID`。`InputPeerSelf` 和显式 `InputPeerUser(self)` 均返回 `Updates`、`messages.Messages`、`messages.SavedDialogsSlice`、`messages.SavedDialogs`，非空消息 ID 分别为 336、337。
- 两次探针均在 finally 中删除本次消息；生产 MySQL 精确回查显示四条历史探针行均为 `deleted=1`、`saved_peer_type=2`、`saved_peer_id=136907713`，没有 active 临时消息。该证据覆盖当前用户保存消息的非空读回和清理，不代表跨用户、频道保存历史或重启恢复。
- r16 镜像仅替换 `biz`：`teamgram-server-latest:20261006-r16-prod`，`biz-r26` SHA-256 `a8f03056e6a470b6070ce18bf341b816fa8b834f8761aef9a3d30cfcb9fbedbd`；`backend-backend-1` 仍 `running`、`restart=0`、`oom=false`。MySQL、Redis、Kafka、etcd 未重启。`go test ./app/service/biz/message/internal/core`、`git diff --check` 通过。

## 2026-10-06 r15 生产授权、联系人要求与壁纸读取复核

- 目标为 `backend-backend-1`。本轮使用当前生产授权经 DC2 WebSocket -> gateway -> session -> BFF -> 各自权威服务 -> 生产 MySQL 完成真实 RPC；未重启数据库、Redis、Kafka、etcd 或其它服务，也未记录或回显授权密钥。
- `account.getAuthorizations` 返回 typed `account.Authorizations`：用户 `136907713` 有 1 条授权、用户 `136907714` 有 3 条授权；每个列表均有 1 条当前授权。TTL 分别为 `180` 和 `0`，与 users 表中的状态一致。授权数量与 `auth_users` 的 active 行数一致。
- `users.getRequirementsToContact` 先返回 `RequirementToContactEmpty`，再在可逆地开启目标账号 premium 联系要求后返回 `RequirementToContactPremium`。探针删除临时隐私行并恢复原设置，生产 MySQL 复核没有遗留临时状态；相邻 `users.getIsPremiumRequiredToContact` 原始 Layer 请求也返回 `true`。
- 新增只读探针 `docker/production/probe-wallpapers-readonly.ts`。两个生产用户的 `account.getWallPapers(hash=0)` 均返回 typed `account.WallPapers` 空列表、零文档；带返回 hash 重试均返回 `account.WallPapersNotModified`。MySQL 只发现一个空的 saved-list 行且没有 uploaded wallpaper 行，因此本轮证明真实空状态和 hash 分支，不证明非空 DFS 文档或全局壁纸目录/provider。
- 生产容器现场保持 `running`、`restart=0`、`oom=false`；当前 `/app/bin/bff` SHA-256 为 `cccf268e97576ca2327e6707f80c38a1ed12051b44a79ecdd9b42da93c003093`。本轮没有改变生产服务状态。

## 2026-10-06 r15 只读配置与保存对话合约复核

- `docker/production/probe-readonly-contracts.ts` 使用用户 `136907713` 的现有生产授权，经 DC2 WebSocket -> gateway -> session -> BFF 完成 9 个只读调用：`help.getAppConfig` -> `help.AppConfig`、原始 `help.getUserInfo#38a08d3` -> `help.UserInfo`、`help.getPeerColors` / `help.getPeerProfileColors` -> `help.PeerColors`、`messages.getAttachMenuBots` -> `AttachMenuBots`、`messages.getPinnedSavedDialogs` -> `messages.SavedDialogs`、`messages.getSavedDialogs` -> `messages.SavedDialogsSlice`、`messages.getSavedHistory(InputPeerSelf)` -> `messages.Messages`、原始 `messages.getSavedDialogsByID#6f6f9c96` -> `messages.SavedDialogs`。
- `getPinnedSavedDialogs`、`getSavedDialogs`、`getSavedHistory` 和 `getSavedDialogsByID` 返回的是当前生产账号的合法空保存对话状态；本轮没有创建保存消息或置顶项，因此没有覆盖非空消息/用户/频道 hydration。`getAttachMenuBots` 返回空 bot/user 向量，不能替代 Bot registry/API 验收。
- 客户端运行时没有生成 `help.getUserInfo` 和 `messages.getSavedDialogsByID` 构造器，探针按 TL constructor 直接编码，证明传输包装和服务端分发路径，而不是把它们误判为未实现。配置与颜色结果分别是静态空 JSON、内置颜色目录，动态 provider 仍未接入。
- 探针只读、无生产写入；执行后容器仍 `running`、`restart=0`、`oom=false`。这些结果把 7 项从 route/component 证据提升为真实 session 链路的部分验收，完整非空业务流程和外部 provider 仍保持未完成。

## 2026-10-06 r24 真实会话读取验收

- `backend-backend-1` 正在运行 `/app/bin/bff-r24`，SHA-256 与 r24 部署记录一致；本轮只做只读 RPC 和 MySQL 查询，没有改生产数据或重启服务。
- 使用既有生产授权经 DC2 WebSocket -> gateway -> session -> BFF -> biz user -> MySQL 调用 `account.getContentSettings`，返回 `account.ContentSettings`，`sensitive_enabled=false`、`sensitive_can_change=true`。生产 `user_settings` 中没有该用户的覆盖行，与 User 服务默认值一致；未执行对应写方法。
- `messages.getSearchResultsCalendar` 用 `PhotoVideo` filter 查询 5 个现有用户/群 peer，均返回 typed `SearchResultsCalendar` 空结果；同一 peer 的 `messages.search` 数量与 calendar count、period count 一致。该结果仅验收真实链路和空状态，非空媒体分桶及 channel/saved-peer 仍未验收，因此不提升该方法状态。
- `messages.getSearchResultsCalendar` 的首次 `InputMessagesFilterEmpty` 请求得到 `SEARCH_QUERY_EMPTY`，符合无搜索词的空 filter 校验；改用协议支持的 `PhotoVideo` filter 后正常返回。服务端代码未因此改动。
- 生产方法台账仍有 813 个唯一 Layer 229 方法；`PARTIAL_ISOLATED_SESSION_E2E=230`、`BLOCKED_BY_IMPLEMENTATION_GAP=289`。`account.getContentSettings` 提升一项；媒体日历保留原验收级别。完整生产级验收仍未完成。

## 2026-10-06 r24 `channels.checkSearchPostsFlood` 生产限额验收

- 当前工作树以 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` 交叉编译 BFF，部署到 `backend-backend-1:/app/bin/bff-r24`，SHA-256 `c1a868c612e0cf12b61c8a3214fb72a15afff934a3ea25a97f200d36712ceaa2`；r23 保留为 `/app/bin/bff-r23.before-r24`。只替换 BFF 进程（PID `2258`），数据库、Redis、Kafka、etcd、authsession、msg、session 和其它服务没有重启。
- `channels.checkSearchPostsFlood` 已移除固定常量，改用配置的 `TotalDaily=10` 和 Redis Lua `INCR+EXPIRE`，按用户 ID 与 UTC 日期分桶；Redis/配置缺失时失败关闭。
- 真实生产授权经 DC2 WebSocket → gateway → session → BFF → Redis 连续调用 11 次，返回 `SearchPostsFlood`：前 10 次 `remains=9..0` 且 `query_is_free=true`，第 11 次 `query_is_free=false` 并返回次日 UTC `wait_till`。探针计数键已删除并确认不存在。该验收只证明限额状态和协议编码，实际频道搜索、Stars 付费 provider 和跨实例并发压力仍未覆盖。
- 台账 `channels.checkSearchPostsFlood` 从实现阻塞提升为 `PARTIAL_ISOLATED_SESSION_E2E`；全量 Layer 229 仍不是 813/813。

## 2026-10-06 r20 BFF 部署与 WebBrowser / LangPack 生产 session 复验

- 从当前工作树构建 Linux ARM64 BFF，SHA-256 `a23426344cd3289a4251c0e3127c54d647cbcc6112f54fd99135d5f66737f2d9`，部署为 `backend-backend-1:/app/bin/bff-r20`。仅替换 BFF 进程为 PID `1575`（容器内）；旧 BFF 精确保留为 `/app/bin/bff.before-20261006-r20`，SHA-256 `6c4aeff3ecca05548ca849ee70e5e4660a8aad64683ee4a53ad4f51a51f3af3a`。容器仍为 `running`、`restart=0`、`oom=false`，镜像仍是 `teamgram-server-latest:20261005-r13-prod`；authsession、msg、session、数据库、Redis、Kafka、etcd 和其它服务没有重启。
- 使用生产 MySQL 已登记用户 `136907713` 的永久授权，通过真实客户端 -> DC2 WebSocket -> gateway -> session -> r20 BFF -> 生产 MySQL 完成验收。授权材料只在本地探针进程中使用，没有写入审计、台账、镜像或日志。
- 浏览器设置流程返回 `account.WebBrowserSettings`；同一 hash 返回 `account.WebBrowserSettingsNotModified`。`account.updateWebBrowserSettings` 的两项布尔设置被切换并由 RPC 与生产 `apifull_kv` 直接读回。`account.toggleWebBrowserSettingsException` 新增临时域名例外返回 `Updates`，RPC/MySQL 均读回；探针随后删除例外并恢复原设置。初始 MySQL key 不存在，探针退出钩子精确删除本次生成的唯一 key，最终 `apifull_kv` 查询计数为 0；客户端返回状态和 MySQL 前后值一致。
- r20 LangPack 真实链路返回：`langpack.getLangPack` -> `LangPackDifference`（3064 条）；`getLanguages` -> 单项 `en`；`getLanguage` -> `LangPackLanguage`；`getStrings` 对两个实际键返回 2 条、重复键去重；`getDifference` 从 version 0 返回 3064 条，从当前 version 1 返回 0 条。英文目录与 `/Users/mac/open/test/telegram-tt-master/src/assets/localization/fallback.strings` 字节一致，并随 BFF 构建嵌入。
- 验证通过：`go test ./app/bff/files/internal/core -count=1`、`go test ./app/bff/authorization/internal/core -run 'TestAuthSignUp' -count=1`、`go test ./app/bff/webbrowser/... -count=1`、`go test ./app/bff/dialogs/internal/core -count=1`、`go test ./app/bff/messages/internal/core -count=1`、`go test ./app/bff/userchannelprofiles/internal/core -count=1`、r20 Linux ARM64 BFF 构建、上述生产探针和 `git diff --check`。APIFull core 的本地 langpack focused test 因 TestMain 强制要求未运行的隔离库 `127.0.0.1:13306/teamgram_audit` 而未通过启动门；没有改用生产库伪装隔离测试。
- 本次真实会话证据只提升 3 个 WebBrowser 写/读方法及 5 个 LangPack 方法；`account.deleteWebBrowserSettingsExceptions` 未调用，仍保持原状态。当前容器仍运行 r13 镜像加 BFF r20 热替换，不等同于完整镜像发布；完整 813 方法生产验收仍未完成。

## 2026-10-06 r20 双用户主链路和运行日志复核

- 使用生产 MySQL 已登记的两个用户授权重新执行双用户真实客户端流程，tag `prod-chain-1791265903772-64325`。DC2 WebSocket -> gateway -> session -> BFF r20 -> msg/inbox/APIFull -> 生产 MySQL -> 客户端返回链路通过，覆盖私聊发送/搜索/已读/编辑/反应、群组与投票、定时消息，以及原生频道成员/消息/阅读数/差分/编辑/删除；返回类型与请求相符，删除后按 ID 查询得到预期 `MESSAGE_ID_INVALID`，探针没有观测到业务 500、超时或连接断开。
- 生产 MySQL 按本轮 tag 回查：8 条探针消息均已删除；临时群为 `deactivated=1`；临时频道及频道消息为 0。清理后没有可见测试消息或活跃临时频道残留。
- 当前容器进程仍为 BFF r20（PID `1575`，SHA-256 `a23426344cd3289a4251c0e3127c54d647cbcc6112f54fd99135d5f66737f2d9`），容器仍运行 r13 镜像、`restart=0`、`oom=false`。本次未重启 BFF 以外的进程、数据库或其它服务；此记录是全链路复验，不代表完整镜像发布。
- 读取最近 45 分钟容器日志并结构化解析 4,346 条 JSON 记录，级别为 `stat=1,361`、`debug=2,035`、`info=950`；没有 `error`、`fatal`、`panic` 级别记录，也没有匹配到明确 HTTP 500 或 deadline/timeout 的日志模式。日志窗口检查不能替代 813 个方法逐项验收。
- 当前方法台账保持 813 行、813 个唯一方法；生产列计数为 `PARTIAL_ISOLATED_SESSION_E2E=222`、`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_COMPONENT_ONLY=157`、`NOT_ACCEPTED=72`、`PARTIAL_ISOLATED_DB_ONLY=43`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整 Layer 229 生产级验收仍未达到 813/813。

## 2026-10-06 r19 全链路服务二进制复验

- 当前工作树有变更的四个运行服务已统一交叉编译并热替换到 `backend-backend-1`：BFF `/app/bin/bff-r19` PID `1254` SHA-256 `6c4aeff3ecca05548ca849ee70e5e4660a8aad64683ee4a53ad4f51a51f3af3a`，authsession `/app/bin/authsession-r19` PID `1428` SHA-256 `26995ebfc525855d83d3d7cb84071ee44539f25373a0b15027a5b1fac5b0e51c`，msg `/app/bin/msg-r19` PID `1448` SHA-256 `aea12d2a6d84e23a90ee23c4fdb56f8e65cbdd9ad721edeccadb91eef5ca6e6c`，session `/app/bin/session-r19` PID `1466` SHA-256 `ca02ed3d2654eb1512fd0336ffb90a9f632b5b2d066d809dbf8070f63d50da7f`。r18/旧二进制均保留为 `.before-20261006-r19`。只重启这四个进程，容器仍 `running`、`restart=0`、`oom=false`，数据库、Redis、Kafka、etcd 和其它服务没有重启。
- etcd 已登记当前实例：`bff.bff -> 172.26.0.7:20010`、`service.authsession -> 172.26.0.7:20450`、`messenger.msg -> 172.26.0.7:20030`、`interface.session -> 172.26.0.7:20120`。四个进程启动日志均显示连接生产 MySQL/Redis/Kafka/etcd 并进入运行循环。
- 在四个最新进程上重新执行 `probe-real-chain.ts`，tag 为 `prod-chain-1791260215090-29064`；真实客户端 -> gateway -> session -> authsession/BFF -> msg/inbox/biz/APIFull -> 生产 MySQL -> 客户端返回全部通过。覆盖认证/配置、私聊消息搜索/全局搜索/已读/编辑、反应、投票完整读写、basic group 论坛/消息/详情/标题、定时消息，以及原生频道成员/详情/消息/搜索/已读/阅读数/差分/编辑/删除；类型化结果再次包括 `messages.MessageViews`、`updates.ChannelDifference` 和 `MESSAGE_ID_INVALID`。
- 在相同四个最新进程上重新执行 `probe-control-plane.ts`（tag `prod-call-1791260275734-29525`），群通话创建/标题/读取/邀请/加入/离开/销毁全部返回 `Updates` / `phone.GroupCall`；重新执行 `probe-reversible.ts` 的 TTL、通知、联系人、生日、设备、outbox 已读日期、置顶、草稿和对话置顶也全部通过。异步 inbox 读取保留 12 次、250ms 轮询上限，超时仍失败。
- MySQL 精确清理：full-chain tag 共 8 行、active=0；本轮 outbox 探针消息 active=0；临时频道及频道消息为 0；群通话记录为 0；临时群仅保留 `deactivated=1` 删除墓碑。服务测试通过：`go test ./app/service/authsession/... ./app/messenger/msg/inbox/... ./app/interface/session/... -count=1`、BFF dialogs/messages/userchannelprofiles/account/authorization/passkey/qrcode focused suites，以及 `git diff --check`。
- 窗口内没有 `panic`、`fatal`、运行时错误、业务 500、deadline timeout 或 resolver 失败；session 日志中的 `invalid salt` 仅触发协议规定的 `bad_server_salt` 恢复后继续处理业务 RPC。该部署仍不是完整镜像重建，且 813 个方法中未具备短信/邮件 provider、支付结算、Passkey/QR trusted registry、第二 DC、原生频道媒体、礼物结算、通话媒体 relay 或秘密聊天客户端明文语义的项目继续保持未验收。

## 2026-10-06 r19 当前源码部署后复验

- 当前 `backend-backend-1` 仅热替换 BFF 到 `/app/bin/bff-r19`，PID `1254`，SHA-256 为 `6c4aeff3ecca05548ca849ee70e5e4660a8aad64683ee4a53ad4f51a51f3af3a`；r18 保留为 `/app/bin/bff.before-20261006-r19`，SHA-256 为 `7d6e96cf6f3537ae7bdc2fb803e13ca8c771dd43cdf19ffe5ed47f759e795ad0`。容器仍为 `running`、`restart=0`、`oom=false`，镜像仍为 `teamgram-server-latest:20261005-r13-prod`；MySQL、Redis、Kafka、etcd、session、msg 和其它服务未重启。
- r19 注册到 etcd 的 `bff.bff/1299184035675147381 -> 172.26.0.7:20010`，BFF 双栈监听已核对。当前源码的 dialogs/messages/user-channel focused tests、account/authorization/passkey/QR tests 和 `git diff --check` 均通过。
- 使用生产 MySQL 当前登记的永久授权，仅在本地探针进程环境变量中注入，未写入仓库、镜像、台账或日志。单用户 `connect -> getMe -> help.getConfig -> photos.getUserPhotos -> account.getBusinessChatLinks` 对用户 `136907713` 返回 `Config`、`photos.Photos`、`account.BusinessChatLinks`。
- r19 上 `probe-real-chain.ts` tag `prod-chain-1791259615702-27954` 通过真实客户端 -> DC2 WebSocket -> gateway -> session -> BFF -> 各服务 -> 生产 MySQL，覆盖认证/配置、私聊发送/历史/本地与全局搜索/已读/编辑、反应、公开投票完整读写、basic group 论坛/消息/详情/改标题、定时消息创建/读取/取消/投递，以及原生频道创建/邀请/成员/详情/消息/搜索/按 ID/已读/阅读数/差分/编辑/删除。返回类型包括 `messages.MessageViews`、`updates.ChannelDifference`，删除后按 ID 得到 `MESSAGE_ID_INVALID`。
- r19 上 `probe-reversible.ts` 通过 TTL、通知设置、联系人增删、生日列表、设备注册/注销、outbox 已读日期、消息置顶/取消、草稿和对话置顶/取消；本次 `prod-outbox-read-*` 消息已全部标记 `deleted=1`。`probe-control-plane.ts` tag `prod-call-1791259742056-28071` 通过群通话创建、标题编辑、读取、邀请、加入、离开和销毁，返回 `Updates` / `phone.GroupCall`。
- 生产 MySQL 精确复核：r19 full-chain tag 8 行均 `active=0`，reversible outbox tag 2 行均 `active=0`，临时频道及频道消息为 0，群通话记录为 0，临时 basic group 只保留 `deactivated=1` 删除墓碑。服务健康仍为 `restart=0`、`oom=false`。
- 当前 `LAYER229_METHOD_LEDGER.csv` 仍为 813 行、813 个唯一方法；生产列为 `PARTIAL_ISOLATED_SESSION_E2E=214`、`BLOCKED_BY_IMPLEMENTATION_GAP=295`、`PARTIAL_COMPONENT_ONLY=160`、`NOT_ACCEPTED=72`、`PARTIAL_ISOLATED_DB_ONLY=43`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。这证明当前部署覆盖的方法链路可验收，但不等于 813/813 生产完成。

## 2026-10-06 r18 生产双用户全链路续验

- 目标容器为 `backend-backend-1`，镜像仍为 `teamgram-server-latest:20261005-r13-prod`；正在运行的 BFF 为 `/app/bin/bff-r18`，PID `1092`，SHA-256 为 `7d6e96cf6f3537ae7bdc2fb803e13ca8c771dd43cdf19ffe5ed47f759e795ad0`。容器保持 `running`、`restart=0`、`oom=false`；本轮没有重启 MySQL、Redis、Kafka、etcd、session、msg 或其它服务。
- 从生产 MySQL 当前登记的永久授权临时注入本地 GramJS 探针，未写入仓库、镜像、台账或日志。单用户链路对 `136907713` 和 `136907714` 均完成 `connect -> getMe -> help.getConfig -> photos.getUserPhotos -> account.getBusinessChatLinks`，返回 `Config`、`photos.Photos` 和 `account.BusinessChatLinks`；两用户连接均无超时。
- `docker/production/probe-real-chain.ts` 的 tag 为 `prod-chain-1791255991951-20944`。真实客户端经 DC2 WebSocket -> gateway -> session -> BFF -> msg/inbox、biz、APIFull 和生产 MySQL 完成认证/配置、私聊发送/历史/本地与全局搜索/已读/编辑、反应、公开投票双方投票/结果/投票者/追加与删除答案、basic group 论坛/消息/详情/改标题、定时消息创建/按 ID/取消/强制投递，以及原生频道创建/邀请/成员/完整信息/消息/搜索/按 ID/已读/阅读数/频道差分/编辑/删除。返回均为预期 typed constructor，包括 `messages.MessageViews`、`updates.ChannelDifference` 和删除后的 `MESSAGE_ID_INVALID`。
- `probe-control-plane.ts` 的 tag 为 `prod-call-1791256141192-21489`，真实完成 `phone.createGroupCall`、标题编辑、读取、邀请、加入、离开和销毁，返回 `Updates` / `phone.GroupCall`；媒体 relay、SCTP/RTMP 仍不在此验收范围。
- `probe-reversible.ts` 在异步投递轮询修复后通过 TTL、通知设置、联系人增删、生日列表、设备注册/注销、outbox 已读日期、临时消息置顶/取消、草稿和对话置顶/取消。探针 tag `prod-outbox-read-1791256354103-22104` 的两条消息均为 `deleted=1`；发送方和接收方均完成读回。
- 同轮通过 `probe-library-reversible.ts`（Saved GIF 写入/读回/清理及 effects/recent-reactions 类型化结果）、`probe-multi-media.ts`（tag `prod-multi-media-1791256801430-23009`，分组媒体双方读回和删除）、`probe-business-profile.ts`（五项业务资料写入、MySQL 回读和清零）、`probe-device-locked.ts`、`probe-suggested-dialog-filters.ts` 和 `probe-saved-reaction-tags.ts`。业务资料的 `main_tab` 已恢复原值，保存反应标签已删除。
- 生产 MySQL 精确清理核对：本轮 full-chain tag 共 8 行且 active=0、outbox tag 共 2 行且 active=0、multi-media tag 共 4 行且 active=0；对应临时频道和频道消息为 0，通话记录为 0，临时 basic group 仅保留 `deactivated=1` 删除墓碑。服务日志显示 `msg.sendMessageV2`、`INSERT messages`、inbox Kafka 投递、B 端 `messages.getHistory` 和 BFF typed reply 均实际发生。
- `probe-reversible.ts` 原先只在发送后立即读取一次，因 inbox 异步投递出现误报；现改为最多 12 次、每次 250ms 的真实 `messages.getHistory` 轮询，仍在超时后失败。定向 Go 测试、Passkey/QR/授权测试和 `git diff --check` 均通过。新连接日志中的 `invalid salt: 0` 均收到 `bad_server_salt` 后继续完成业务 RPC，窗口内没有业务 500、panic、deadline timeout 或连接断开。
- 本轮仍不是 813/813 生产完成。短信/邮件 provider、真实支付结算、可信 Passkey/QR registry、第二 DC、多 DC 转移、原生频道媒体/CDN、礼物库存结算、通话媒体控制面和秘密聊天客户端明文解密仍缺权威后端或外部配置，继续保持明确 fail-closed；BFF 热替换也不等同于完整镜像发布。

## 2026-10-06 r17 BFF 热替换与保存反应标签/真实链路复验

- 从当前工作树交叉编译 Linux ARM64 BFF，部署到 `backend-backend-1` 的 `/app/bin/bff-r17`，SHA-256 为 `307a8c6a8ee104d0600f7cfa4980d5424e85a1a79a9656bf9cac87db83748edb`，当前 BFF PID 为 `973`；旧版本保留为 `/app/bin/bff.before-20261006-r17`。只替换并重启 BFF 进程，数据库、Redis、Kafka、etcd、session、msg 和其它服务未重启。
- 生产探针 `docker/production/probe-saved-reaction-tags.ts` 完成 `SavedReactionTags -> Boolean -> SavedReactionTags -> SavedReactionTagsNotModified -> Reactions -> Boolean -> SavedReactionTags`，写入的 `🧪` 标签已删除，Redis 无残留。
- 生产真实链路探针 `prod-chain-1791248174862-7665` 经真实客户端、DC2 WebSocket、gateway、session、BFF、业务服务和 MySQL 完成认证、配置、私聊搜索/编辑/已读、反应、投票、群组、论坛、定时消息、频道成员/消息/阅读数/差分/编辑/删除等流程；均返回预期 typed 类型。
- 清理复核：探针消息和临时群均为 0 个 active 记录，删除墓碑仍保留 8 条消息和 1 个群，符合当前删除模型。容器仍为 `running`、`restart=0`、`oom=false`，镜像仍是 `teamgram-server-latest:20261005-r13-prod`。
- 频道阅读数已按保存消息返回 `messages.MessageViews`，不再对真实频道统一返回 `CHANNEL_INVALID`；`channels.searchPosts` 对空 query、负 offset 和未支持的 paid-stars 参数保持明确错误，未伪造全局搜索或支付 provider。
- 本次是 BFF 热替换，不是完整镜像部署；其它服务仍使用生产旧二进制。没有把未覆盖的方法提升为生产级通过，手机号/短信、真实支付、第二 DC、Passkey 完整 ceremony、礼物结算、秘密聊天客户端解密和媒体 relay 仍为未完成或明确 fail-closed。

## 2026-10-06 本轮继续：消息/对话 provider 边界与分页上限

- `messages.getDialogs` 现在把 Layer 229 的最大 `limit=500` 传给下游，不再只在局部变量中截断而仍请求原始数量；新增 501 条请求返回 500 条的 focused test。
- `messages.getHistory` 的 basic-chat 路径和 `messages.search` 的普通/saved-peer 路径现在在 Chat/Message provider 或 helper 缺失时失败关闭；搜索结果 hydration 也检查 User/Chat provider 和 helper 返回，避免 nil 解引用。
- 定向验证通过：`go test ./app/bff/dialogs/... -count=1`、`go test ./app/bff/messages/internal/core -count=1` 和 `git diff --check`。这些修复尚未重新部署到 `backend-backend-1`，因此不提升现有生产验收状态。
- 当前台账仍为 813 个唯一方法、每行 8 个字段；生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=295`、`PARTIAL_ISOLATED_SESSION_E2E=214`、`PARTIAL_COMPONENT_ONLY=160`、`NOT_ACCEPTED=72`、`PARTIAL_ISOLATED_DB_ONLY=43`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。实现阻塞项主要仍是缺少短信/邮件、支付结算、可信 Passkey/QR 多 DC、原生频道 provider、贴纸/机器人、礼物库存结算和通话媒体后端；这些保持明确失败语义。

## 2026-10-06 资料方法边界校验

- `photos.getUserPhotos` 现在在 User/Media 权威服务返回的照片 ID 上执行 `offset`、`max_id` 和最多 100 项的 `limit` 窗口，负 offset 返回 `OFFSET_INVALID`；新增分页和错误路径测试。
- `account.updatePersonalChannel` 现在在调用 User 持久化 RPC 前校验认证、请求、频道对象和正频道 ID；`photos.uploadContactProfilePhoto` 在媒体服务返回空对象或无效 ID 时失败关闭，避免写入无效联系人照片引用。
- 组件测试通过后，已将 Linux ARM64 BFF 热替换到 `backend-backend-1` 的 `/app/bin/bff-r16`（SHA-256 `baf631e8a9b23055579458f337d1eaf38d4fbab184afe7e088e6029f8f289e68`，PID `785`）；r15 保留为 `/app/bin/bff-r15`。容器仍 `running`、`restart=0`、`oom=false`，只重启 BFF 进程。
- BFF RPC 端口 `20010` 仍可连接，启动日志完成 MySQL/etcd/KV 客户端初始化且无 panic/fatal；本轮没有执行生产数据库写入，也没有把这些方法提升为生产会话验收。
- `messages.editChatPhoto` 增加了认证、请求、聊天/媒体/消息依赖和空响应校验，避免空指针或无效照片进入消息服务；新增 invalid-request focused tests，仍未宣称真实群权限和消息同步已验收。

## 2026-10-06 r15 BFF 热替换与生产 suggested filters 验收

- 从当前工作树交叉编译 Linux ARM64 BFF，SHA-256 为 `030c893b1372dcd9b5074b5e08d08662db9cc2427a9b4e2c32f4881fb0819b55`，在 `backend-backend-1` 内以 PID `418` 启动 `/app/bin/bff-r15`。只替换 BFF 进程，session、msg、数据库、Redis、Kafka、etcd 和其它服务没有重启；容器仍 `running`、`restart=0`、`oom=false`。
- 部署后真实客户端通过 DC2 WebSocket → gateway → session → BFF 调用 `messages.getSuggestedDialogFilters`，返回 3 个 typed `DialogFilterSuggested`，标题为 Contacts、Groups、Channels；该方法已提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。
- 这是当前源码首次在生产容器中验证；r14 仍保留在 `/app/bin/bff-r14`，可回滚。该方法没有写库，未产生清理残留；r15 其它未覆盖方法仍不能按部署成功推断。
- 当前台账仍为 813 个唯一方法，生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_ISOLATED_SESSION_E2E=209`、`PARTIAL_COMPONENT_ONLY=176`、`NOT_ACCEPTED=63`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。
- r15 部署后重跑完整真实链路 tag `prod-chain-1791233101899-32105`，消息、反应、投票、论坛、定时消息、频道成员/消息/阅读数/差分均保持预期类型；生产清理后 `prod-chain-%` active messages、`prod-call-%` group calls 和临时群均为 0。

## 2026-10-06 r15 生产论坛话题写入复验

- 真实客户端在临时超级群/论坛 `1791233315773726254` 中创建两个话题，调用 `messages.editForumTopic` 修改标题、图标、关闭和隐藏字段，调用 `messages.updatePinnedForumTopic` 置顶并读回，再调用 `messages.reorderPinnedForumTopics` 调整顺序并读回；所有写入返回 `Updates`，读回返回 `messages.ForumTopics`。
- 探针结束后删除临时频道，生产库确认该频道已停用且 `prod-forum-%` active messages=0。三项方法已提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`；多角色授权、跨会话更新和重启持久性仍未覆盖。

## 2026-10-06 生产状态复核（backend-backend-1 r14）

- 当前生产容器 `backend-backend-1` 仍为 `running`、`restart=0`、`oom=false`；BFF 进程为 PID `262`，二进制 SHA-256 为 `6494eef15dd841509fd493e0c92911381bb71040b879944ee652e5152b8373e8`。本轮没有重启数据库、session、msg、Redis、Kafka、etcd 或其它服务。
- 生产 MySQL 清理复核：`prod-%` 活跃消息为 0，`prod-%` 群通话为 0，用户 `136907713` 的探针 KV 为 0，`users.main_tab` 已恢复为 `0`。临时反应群 `120074`、`120075`、`120077` 已停用；清理后的反应字段为空。
- 重新使用生产库中的既有授权，通过 `telegram-tt-master` 真实客户端完成双用户流程：创建临时群、设置 `ChatReactionsSome(👍)`、经 `messages.getFullChat` 读回、查询 MySQL 后清空并停用。非空写入时 `HEX(available_reactions)=5B22F09F918D225D`，确认数据库保存的是 UTF-8 `["👍"]`，不是问号替代字符；`messages.setChatAvailableReactions` 已提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。
- 当前 `LAYER229_METHOD_LEDGER.csv` 仍为 813 个唯一方法，生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_ISOLATED_SESSION_E2E=205`、`PARTIAL_COMPONENT_ONLY=176`、`NOT_ACCEPTED=67`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整 Layer 229 仍不是生产级 813/813。
- `git diff --check` 和 `go test -c ./app/bff/apifull/internal/core -o /tmp/apifull-core-current.test` 通过；APIFull 运行时测试需显式隔离 DSN `127.0.0.1:13306/teamgram_audit`，该端口本轮未启动，未使用生产库运行测试。

## 2026-10-06 生产双用户可逆探针复验

- 使用生产用户 `136907713` / `136907714` 的既有授权，经 `telegram-tt-master` GramJS 客户端和 DC2 WebSocket 完成 `account` TTL、通知设置、联系人增删、生日联系人、设备注册/注销、草稿、对话置顶、私聊已读日期、临时消息置顶/取消置顶等流程。
- 关键返回类型为 `AccountDaysTTL`、`PeerNotifySettings`、`Updates`、`contacts.ContactBirthdays`、`OutboxReadDate`、`messages.AffectedHistory` 和 `messages.PeerDialogs`；临时消息、联系人、草稿、置顶和设备 token 均在探针结束时恢复/删除。该轮 `messages.getOutboxReadDate` 的回读消息 ID 为 `241`，没有残留 active 探针消息。
- 本轮没有重启任何服务；该证据强化了真实客户端到生产数据库的可逆链路，但不覆盖 provider 结算、跨 DC、重启后持久性或未在探针中的方法。

## 2026-10-06 生产完整真实链路探针复验

- `docker/production/probe-real-chain.ts` 使用同一批生产授权，经真实客户端 → DC2 WebSocket → gateway → session → BFF → 业务服务 → 生产 MySQL 完成，tag 为 `prod-chain-1791230978680-16926`。认证、配置、国家/时区、反应目录、Saved GIF、对话、直接消息搜索/编辑/已读、反应、投票、论坛话题、定时消息和 APIFull 频道全链路均返回预期类型。
- 真实返回包括 `User`、`Config`、`NearestDc`、`help.CountriesList`、`help.TimezonesList`、`messages.DialogsSlice`、`messages.Messages`、`messages.AffectedHistory`、`messages.VotesList`、`messages.ForumTopics`、`messages.ChatFull`、`channels.ChannelParticipant`、`channels.ChannelParticipants`、`messages.MessageViews` 和 `updates.ChannelDifference`。频道阅读数为 `messages.MessageViews`，删除后按 ID 读取得到预期 `MESSAGE_ID_INVALID`。
- 探针完成后生产库核对：`prod-chain-%` active messages=0、`prod-call-%` group calls=0、`prod-chain-%` 未停用临时群=0；容器仍 `running`、`restart=0`。这证明本轮覆盖的方法经过真实生产链路和持久化清理，但不等同于 813 个方法全部验收。

## 2026-10-06 生产 grouped media 流程复验

- 真实双用户客户端通过 DC2 WebSocket 发送两项 `messages.sendMultiMedia` 联系人媒体，接收方通过 `messages.Messages` 读回两条消息并确认 grouped 结果，随后发送方和接收方分别用 `messages.deleteMessages` 清理，均返回 `messages.AffectedMessages`。
- 探针 tag 为 `prod-multi-media-1791231375027-17517`；生产库清理后没有保留该 tag 的 active 消息。本轮只覆盖现有 APIFull 媒体路径，不代表原生频道媒体/CDN provider 已完成。

## 2026-10-06 生产群通话控制面复验

- 补齐真实成员 access hash 后，`probe-control-plane.ts` 通过生产双用户 WebSocket 完成 `phone.createGroupCall`、`phone.editGroupCallTitle`、`phone.getGroupCall`、`phone.inviteToGroupCall`、`phone.joinGroupCall`、`phone.leaveGroupCall` 和 `phone.discardGroupCall`，返回 `Updates` / `phone.GroupCall`。
- tag 为 `prod-call-1791231737687-29895`；探针销毁临时通话，生产库 `prod-call-%` active group calls 仍为 0。媒体 relay、SCTP/RTMP 和 Stars 结算控制面仍未覆盖。

## 2026-10-06 生产秘密聊天跨设备复验

- 真实客户端经生产 DC2 WebSocket 完成 `messages.getDhConfig`、`messages.requestEncryption`、`messages.acceptEncryption`（同一用户两台设备）、`messages.sendEncrypted`、`updates.getDifference`、重连后的 `messages.sendEncrypted` / `messages.readEncryptedHistory` 和 `messages.discardEncryption`。
- 结果为 `messages.DhConfig`、`EncryptedChatWaiting`、两次 `EncryptedChat`、`messages.SentEncryptedMessage`、`updates.Difference` 和 `Boolean`；共享密钥相等、fingerprint 匹配、第二设备旧 key 重试没有覆盖新 key，qts 从 4 增至 5，密文逐字节匹配。
- 临时 chat `1791232053` 被标记丢弃后，按双方用户、chat ID 和 `history_deleted=1` 精确删除了 3 条 device-key、chat 行和 0 条消息；用户级 qts 没有回退。销毁阶段的 WebSocket close 日志不影响上述 RPC 结果。

## 2026-10-06 无外部依赖的 emoji 语言目录补齐

- `messages.getEmojiKeywordsLanguages` 已从 `METHOD_NOT_IMPL` 改为返回类型化 `Vector<EmojiLanguage>`。目录只声明 APIFull 当前关键词接口支持的 `en` 基线，按请求顺序去重；未支持语言和空请求返回空向量，未认证仍返回 `AUTH_KEY_UNREGISTERED`。
- 聚焦测试和 APIFull 核心编译通过。该方法尚未部署到 `backend-backend-1`，也没有真实会话读回，因此生产列只提升为 `PARTIAL_COMPONENT_ONLY`，不计入生产级通过。

## 2026-10-06 无外部依赖的浏览器设置补齐

- `account.getWebBrowserSettings`、`account.updateWebBrowserSettings`、`account.toggleWebBrowserSettingsException`、`account.deleteWebBrowserSettingsExceptions` 已确认走现有 per-user 持久化路径：设置项、外部/应用内域名例外、hash not-modified 和清空语义均有组件测试覆盖。
- `go test ./app/bff/webbrowser/internal/core -run 'TestWebBrowserSettings' -count=1` 通过。实现尚未部署到 `backend-backend-1`，所以台账仅为 `PARTIAL_COMPONENT_ONLY`，不计入生产级通过。

## 2026-10-06 无外部依赖的快捷回复和保存反应标签补齐

- `messages.reorderQuickReplies`、`messages.checkQuickReplyShortcut` 已用组件测试确认 caller-scoped 读写、排序去重、未知 ID 保留策略和名称可用性返回；现有快捷回复发送、删除和消息 hydration 测试继续通过编译。
- `messages.getSavedReactionTags`、`messages.updateSavedReactionTag` 已用组件测试确认 emoji 标签新增、删除、hash not-modified、非法 reaction 和未认证边界。
- `messages.getDefaultTagReactions` 已确认无保存标签时返回内置静态 emoji，存在保存标签时返回 caller-scoped 标签集合。
- `bots.setBotCommands`、`bots.resetBotCommands`、`bots.getBotCommands`、`bots.setBotMenuButton`、`bots.getBotMenuButton` 的现有组件测试已覆盖 caller-scoped 持久化和 typed readback；它们仍没有 Bot ownership 或 Bot API 投递链路。
- chatlist 导出、查询、重命名、删除和隐藏方法的现有生命周期测试已覆盖 caller-owned 持久化、频道实体 hydration 与隐藏后的空更新；chatlist 加入/离开、跨用户分享和生产会话仍未验收。
- APIFull 核心编译通过；完整 `go test` 仍受隔离数据库前置条件限制（必须存在 `127.0.0.1:13306/teamgram_audit`）。这些方法尚未部署到 `backend-backend-1`，台账只记为 `PARTIAL_COMPONENT_ONLY`。
- CSV 当前生产列计数：`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_COMPONENT_ONLY=170`、`PARTIAL_ISOLATED_SESSION_E2E=204`、`PARTIAL_ISOLATED_DB_ONLY=46`、`NOT_ACCEPTED=74`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 `account.setMainProfileTab` 生产写入复验

- 真实授权会话经 DC2 WebSocket -> gateway -> session -> UserChannelProfiles BFF -> User service 调用原始 Layer 229 请求，返回 `Boolean`。
- 生产 MySQL `users.main_tab` 读回从 `0` 变为 `1`（`profileTabPosts`），随后恢复为原值 `0`；没有留下用户状态变化。GramJS runtime 没有该请求的生成构造器。
- 该方法从 `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。跨会话更新和重启持久性仍未验收。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=291`、`PARTIAL_ISOLATED_SESSION_E2E=204`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=88`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 无外部依赖的本地功能补齐

- `messages.getSuggestedDialogFilters` 已补齐内置联系人、群组和频道三种 `DialogFilterSuggested` 模板。模板不包含伪造用户、聊天或频道，客户端可以直接保存或编辑；未认证请求仍返回 `AUTH_KEY_UNREGISTERED`。
- APIFull 核心编译通过，聚焦测试已改为验证三种 typed suggestion。当前生产容器尚未替换为包含该实现的二进制，因此台账生产列暂保持 `NOT_ACCEPTED`，部署后需通过真实 WebSocket 再提升。

## 2026-10-06 saved reaction tags 生产可逆复验

- 使用生产 MySQL 中已登记的用户 `136907713` 永久授权，仅在单次本地探针进程环境变量中注入；没有打印、写入仓库或写入台账。目标为 `backend-backend-1` 的当前 r13 BFF，探针经 `127.0.0.1:11443` 完成真实 DC2 WebSocket → gateway → session → BFF → Redis 链路。
- `docker/production/probe-saved-reaction-tags.ts` 先读取现有标签，再写入唯一 `🧪` 标签，读回 `messages.SavedReactionTags`，用返回 hash 得到 `messages.SavedReactionTagsNotModified`，通过 `messages.getDefaultTagReactions` 读回 `messages.Reactions`，最后删除并确认标签消失；若预先存在同一标签则恢复原标题。
- 本轮结果为 `SavedReactionTags`、`Boolean`、`SavedReactionTagsNotModified`、`Reactions`、`Boolean`、`SavedReactionTags`，没有 400、500、panic、超时或连接断开。生产方法台账中的三个 saved reaction tag 方法已从 route-only/not-accepted 提升为 `PARTIAL_ISOLATED_SESSION_E2E`；跨会话更新投递、重启持久性和完整 custom emoji 媒体解析仍未验收。
- 没有重启 `backend-backend-1`、数据库或其它服务，探针只留下 Redis 原值；生产列计数需以 CSV 结构化解析为准。

## 2026-10-06 业务资料五项生产写入与清理复验（backend-backend-1 r14）

- 仅替换生产容器中的 BFF 进程为 Linux ARM64 r14（SHA-256 `6494eef15dd841509fd493e0c92911381bb71040b879944ee652e5152b8373e8`），session、msg、数据库、Redis、Kafka、etcd 和其它进程未重启；容器 `restart=0`。
- 真实授权会话经 DC2 WebSocket -> gateway -> session -> BFF -> APIFull，对 `account.updateBusinessWorkHours`、`account.updateBusinessLocation`、`account.updateBusinessGreetingMessage`、`account.updateBusinessAwayMessage`、`account.updateBusinessIntro` 发送原始 Layer 229 请求，五项均返回 `Boolean`。
- 生产 MySQL `apifull_kv` 读回五个临时 key 均为非空；发送空请求清理后五个值均为空。`account.updateBusinessLocation` 的空请求清理语义已修复并随 r14 生效。探针没有输出业务内容或凭据。
- GramJS runtime 对这五个请求只有声明、没有生成构造器，因此探针使用原始 TL 编码；业务消费者、客户端生成代码、重启后回读和 `users.getFullUser` 的完整 hydration 仍未验收。
- 五项从 `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=203`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=88`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 `phone.getCallConfig` 生产只读复验

- 真实生产授权会话经 DC2 WebSocket -> gateway -> session -> APIFull 调用 `phone.getCallConfig`，返回 `DataJSON`，JSON 包含音频帧、jitter、视频拥塞窗口和 TURN 配置字段；只记录字段名和长度，没有输出任何凭据或 peer tag。
- 该方法已从仅路由证据提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。TURN relay 连通性、通话媒体协商和重启持久性仍未验收。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=198`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=93`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 `messages.unpinAllMessages` 生产可逆流程

- 在同一真实私聊中创建临时消息并置顶，经 DC2 WebSocket -> gateway -> session -> messages BFF -> Msg provider 读回 `pinned=true`。
- 调用 `messages.unpinAllMessages` 返回 `messages.AffectedHistory`；随后读取历史确认该消息已清除置顶，最后删除临时消息。探针返回无 400、500、超时或 panic，生产容器保持 `restart=0`。
- 该方法已从仅路由证据提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。频道、话题、saved-peer 标志组合和重启持久性仍未验收。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=197`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=94`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 `contacts.getBirthdays` 生产会话复验

- 使用生产 `backend-backend-1` 的真实授权会话，经 DC2 WebSocket -> gateway -> session -> userchannelprofiles BFF -> User/MySQL 调用 `contacts.getBirthdays`，返回 `contacts.ContactBirthdays`，没有 400、500、超时或 panic。
- 当前用户没有生日联系人，返回 `contactCount=0`、`userCount=0`；这证明空结果的类型化编码和 provider 链路，不证明已有生日数据的 hydration。探针未写入业务数据。
- `LAYER229_METHOD_LEDGER.csv` 中该方法从 `NOT_ACCEPTED` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。跨会话更新、重启持久性和真实生日记录仍未验收；完整 Layer 229 仍不是 813/813。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=196`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=95`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。

## 2026-10-06 `account.updateDeviceLocked` 生产可逆写入

- 使用真实生产授权会话通过 DC2 WebSocket → gateway → session → notification BFF 写入 `period=3600`，返回 `Boolean`。生产 Redis 的 `device_locked:136907713` 精确读回 `3600`，随后恢复为探针前不存在状态。
- 当前 GramJS runtime 只有该方法的声明，未导出生成构造器；探针使用相同 constructor `0x38df3532` 和 Layer 229 little-endian `int` 原始请求完成编码。这证明服务端链路，不等同于客户端生成代码已可调用；设备锁定消费者/执行和重启持久性仍未验收。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=195`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=96`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整 Layer 229 仍为 `0/813`。

## 2026-10-06 生产可逆状态与反应清理复验（backend-backend-1）

- `messages.saveDefaultSendAs` 通过真实 DC2 WebSocket → gateway → session → BFF → APIFull → Redis 返回 `Boolean`。用户 `136907713` 为 peer `136907714` 保存自身 send-as，精确读取 `default_send_as:136907713:2:136907714` 得到预期值，随后恢复该 key 的原始不存在状态；频道 roster 权限和重启持久性仍未覆盖。
- `docker/production/probe-reactions-focused.ts` 的 tag `prod-reaction-1791218222976-66203` 发送临时私聊反应，依次返回 `messages.Messages`、`messages.AffectedHistory`、`Boolean`（`messages.clearRecentReactions`）和 `Updates`，随后删除发送方/接收方副本。该方法从仅路由提升为生产会话部分验收。
- 当前生产列计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=194`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=97`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整 Layer 229 仍为 `0/813`。

## 2026-10-05 当前 r13 生产探针复验

- 从生产 `auth_keys` 中读取两名既有用户的永久授权，仅在本地探针进程注入；没有写入台账或日志。`docker/production/probe-real-chain.ts` 通过 `127.0.0.1:11443` 完成 `WebSocket → gateway → session → BFF → 业务服务 → 生产 MySQL`，tag 为 `prod-chain-1791166674875-75064`。
- 本轮返回的类型化结果包括 `User`、`Config`、`NearestDc`、`help.CountriesList`、`help.TimezonesList`、`messages.DialogsSlice`、直接消息搜索/全局搜索/已读/编辑、反应、公开投票及追加/删除答案、未读投票、论坛话题、定时消息和 canonical 频道的成员、频道列表、完整频道、消息、搜索、已读、阅读数、差分、编辑、删除。`channels.getParticipant` 返回 `channels.ChannelParticipant`，`channels.getChannels` 返回 `messages.Chats`，因此两项从隔离数据库级提升为生产会话部分验收。
- 同一轮 `probe-library-reversible.ts` 返回 `messages.SavedGifs`、`messages.AvailableEffects`、`messages.Reactions`，效果目录和最近反应均为空但编码正常；`probe-reversible.ts` 覆盖 TTL、通知设置、联系人、设备注册/注销、outbox 已读日期、草稿和置顶读回；`probe-control-plane.ts` 的群通话控制面也全部返回预期类型。
- 生产 MySQL 清理核对：`prod-chain-*` 活跃消息、canonical 频道/频道消息、临时群和 `prod-call-*` 通话均为 0。完整 Layer 229 仍为 `0/813`；外部 provider、第二 DC、权威目录/结算、媒体控制面等阻塞项不因本轮可逆探针而完成。

## 2026-10-05 `messages.saveDefaultSendAs` 组件边界修复

- `messages.saveDefaultSendAs` 现在在读取请求字段前先拒绝未认证 metadata 和 nil request，避免会话进程因空指针崩溃；有效请求仍按用户、目标 peer 和 send-as peer 写入共享 APIFull KV。
- 新增聚焦测试覆盖 `AUTH_KEY_UNREGISTERED`、`PEER_ID_INVALID` 和 `default_send_as` 持久化回读。该项生产列仍为 `NOT_ACCEPTED`，因为本轮没有使用真实授权会话跑写入/读回或重启持久性探针。
- `contacts.getBirthdays` 现在拒绝未认证或 nil 请求，在 User provider 缺失、返回 nil 或联系人 hydration 出错时失败关闭并传播错误，不再把 provider 错误吞成空的生日列表。新增聚焦测试覆盖认证、请求和 provider 错误路径；真实生产生日数据仍未验收。
- `account.updateDeviceLocked` 现在在持久化前拒绝未认证 metadata 和 nil request，避免通知 BFF 的空指针路径；生产列仍为 `NOT_ACCEPTED`，因为设备锁定执行和真实会话读回尚未验证。
- 论坛方法的隔离 APIFull 测试新增了管理员编辑、置顶、排序后的读回断言，并保留成员/非成员权限拒绝。`messages.editForumTopic`、`messages.updatePinnedForumTopic`、`messages.reorderPinnedForumTopics` 已标记为组件部分验证；本机未配置隔离 `teamgram_audit` DSN，所以这组三个测试本轮未执行，生产列继续为 `NOT_ACCEPTED`。

## 2026-10-05 r13 生产镜像重建复核（backend-backend-1）

- 基于已运行并验证过的 r12 运行时制作 `teamgram-server-latest:20261005-r13-prod` overlay 镜像，仅替换当前工作树编译的 Linux ARM64 BFF；镜像 digest 为 `sha256:919f024e34357091a63134fe9da739ab5c336c1c9b87281011ed705655990e1c`，BFF SHA-256 为 `3a0c71aa48485c13e5e0f4fb64f9b540cab04771e2105c572da132d0709b4184`。
- 使用 `docker/production/backend-deploy-r13-prod.yaml` 仅重建 `backend-backend-1`。容器为 `running`、`restart=0`、`oom=false`，11 个 Teamgram 进程均已启动；MySQL、Redis、Kafka、etcd、SeaweedFS、coturn、identity 和其它容器未重启。
- 重建后真实 DC2 探针再次通过：`messages.deleteHistory` 返回 `messages.AffectedHistory`，生产 MySQL 对应 tag 为 1 条 `deleted=1`、临时群为 1 条 `deactivated=1`；`messages.saveGif`/`messages.getSavedGifs`/`messages.getAvailableEffects`/`messages.getRecentReactions` 返回正确类型并完成清理。随后重跑全链路探针，消息/反应/投票/论坛/定时消息/canonical 频道流程全部保持类型化结果，`prod-chain-*` 消息 8 行为删除墓碑、群 1 行为停用墓碑、频道行无残留。
- 重建后的日志没有 500、panic 或超时；会话首次使用旧 salt 时出现的 `bad_server_salt` 是 MTProto 正常盐刷新，探针清理已删除消息时的 `MESSAGE_ID_INVALID` 是预期幂等校验。
- 复核并按精确 `prod-chain-*`/`prod-delete-history-*`/`prod-no-forwards-*` 前缀清理了 3 个历史探针遗留的活动 basic group；生产 MySQL 最终回读活动消息、活动探针群和活动探针频道均为 0。
- 全链路轮询流程新增真实证据：`messages.addPollAnswer` 返回 `Updates`，`messages.getUnreadPollVotes` 返回 `messages.Messages`，`messages.readPollVotes` 返回 `messages.AffectedHistory`；这些方法已从仅路由状态提升为生产部分验收。随后用 GramJS 缺失构造器的原始 MTProto 编码补测 `messages.deletePollAnswer`，真实群投票答案删除返回 `Updates`，也已提升为生产部分验收。
- 当前 CSV 生产列计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=192`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=99`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。本次新探针 tag 为 `prod-chain-1791166674875-75064`；`channels.getParticipant` 与 `channels.getChannels` 也已通过 canonical 频道的生产 session 链路并完成临时数据清理。`messages.getAvailableEffects` 与 `messages.getRecentReactions` 仍只证明类型化空结果和调用链，不代表已接入权威效果目录或完整最近反应历史；完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 `messages.toggleNoForwards` 生产复核（backend-backend-1）

- 真实用户在生产 basic group 上调用 `messages.toggleNoForwards(enabled=true)` 和 `enabled=false`，两次均返回 `Updates`；`messages.getFullChat` 读回 `noforwards=true`，关闭后 canonical Chat 省略默认 false 字段，确认没有保持开启。
- 该群随后通过 `messages.deleteChat` 清理，生产 MySQL 回读对应 `chats` 行为 `deactivated=1`，没有留下活动群。该证据只覆盖 basic group；频道、迁移群和跨客户端更新仍未验收。
- 当前 CSV 生产列计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_ISOLATED_SESSION_E2E=186`、`PARTIAL_COMPONENT_ONLY=157`、`NOT_ACCEPTED=105`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 `messages.saveGif` Redis 持久化复核（backend-backend-1）

- 发现并修正 APIFull DAO 初始化顺序：生产配置同时提供 MySQL DSN 和 Redis KV 时，原实现只打开 MySQL，跳过 `persist.Use(kv.NewStore(...))`，使 GIF、草稿等 KV 状态退回进程内内存；现在两种存储会同时配置。
- Linux ARM64 BFF SHA-256 为 `3a0c71aa48485c13e5e0f4fb64f9b540cab04771e2105c572da132d0709b4184`，旧副本保留为 `/app/bin/bff.before-20261005-kv-store`。只重启 BFF，容器 `backend-backend-1` 仍为 `running`、`restart=0`、`oom=false`，数据库和其它业务进程未重启。
- 真实 DC2 会话先用唯一 `InputDocument` 调用 `messages.saveGif` 并从 `messages.getSavedGifs` 读回；BFF 重启后再次读回同一文档，随后调用 `messages.saveGif(unsave=true)` 并确认列表清除。Redis 中没有残留 GIF key。
- `messages.getAvailableEffects` 和 `messages.getRecentReactions` 本轮返回了类型化但空的结果（效果目录/最近反应计数均为 0），未把空目录误报为完整 catalogue 实现，继续保留待补权威数据源。
- 当前 CSV 生产列计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_ISOLATED_SESSION_E2E=185`、`PARTIAL_COMPONENT_ONLY=157`、`NOT_ACCEPTED=106`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 当前部署全链路探针复核（backend-backend-1）

- 使用生产用户 `136907713` / `136907714` 的授权会话，通过 DC2 WebSocket → gateway → session → BFF → 业务服务 → 生产 MySQL 重跑 `docker/production/probe-real-chain.ts`。
- 只读与双用户消息流程均返回类型化结果：`Config`、`NearestDc`、`help.CountriesList`、`help.TimezonesList`、`messages.AvailableReactions`、`messages.SavedGifs`、`messages.DialogsSlice`、消息搜索/全局搜索、已读回执、反应、公开投票、论坛话题、定时消息和频道成员/完整信息/消息搜索/阅读数/频道差分/编辑/删除均通过。
- 频道阅读数返回 `messages.MessageViews`；频道差分返回 `updates.ChannelDifference`；删除后按 ID 读取得到预期 `MESSAGE_ID_INVALID`。本轮唯一探针 tag 的生产 MySQL 回读为 `messages` 8 行全部 `deleted=1`，basic group 1 行 `deactivated=1`，`apifull_channel` 与 `apifull_channel_message` 均无残留。
- 服务日志中出现的 `MESSAGE_ID_INVALID` 仅来自探针对已经清理的 scheduled/channel 消息做幂等清理；窗口内没有 500、panic、超时或 resolver 失败。该探针仍只覆盖可逆的双用户、basic group 与 canonical channel 流程，外部 provider、第二 DC、媒体控制面和重启持久性不因此通过。

## 2026-10-05 生产 `messages.deleteHistory` 真实链路复核（backend-backend-1）

- 用生产 MySQL 中现有用户 `136907713` / `136907714` 的授权会话，通过 DC2 WebSocket → gateway → session → BFF → msg → 生产 MySQL 创建临时 basic group 并发送唯一 tag。
- `messages.deleteHistory` 返回 `messages.AffectedHistory`；Alice 和 Bob 随后读取该群历史都不再看到 tag。生产 MySQL 精确回读该 tag 共 3 行，`active=0`、`deleted=3`；临时群 `chats.deactivated=1`，没有留下可见残留。
- BFF 日志显示请求中的 `min_date`/`max_date` 为空 wrapper 被按未设置处理，返回 `pts=384`、`pts_count=3`；探针窗口没有 500、panic、超时或 resolver 错误。
- 本轮修复了 `messages.deleteHistory` 将空日期 wrapper 错判为日期过滤、进而把 basic group 转成错误 peer 类型的问题。该证据只覆盖可逆 basic-group 流程；频道/秘密聊天、真实日期过滤和重启持久性仍未验收。
- 当前 CSV 生产列计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`PARTIAL_ISOLATED_SESSION_E2E=184`、`PARTIAL_COMPONENT_ONLY=157`、`NOT_ACCEPTED=107`、`PARTIAL_ISOLATED_DB_ONLY=46`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 生产多媒体分组消息复核（backend-backend-1）

- `docker/production/probe-multi-media.ts` 通过 DC2 WebSocket → gateway → session → BFF → msg/inbox → 生产 MySQL 发送两条 `InputMediaContact`，Bob 读回两条 `MessageMediaContact`，两条消息共享同一个 `grouped_id`；`messages.sendMultiMedia` 返回 `Updates`，接收方读回为 `messages.Messages`。
- 修正了探针清理错误：此前误把 Bob 副本的行号交给 Alice 的 `messages.deleteMessages`，导致发送方删除路径无法命中。现在探针分别读取发送方和接收方的消息 ID，Alice 以 `revoke=true` 删除发送方并触发 inbox 清理，Bob 再删除自己的副本。修正后的生产复核返回两次 `messages.AffectedMessages`，精确 tag 在生产 MySQL 回读为 `active=0`、`deleted=4`。
- 因此 `messages.sendMultiMedia` 和 `messages.deleteMessages` 从静态/仅路由状态提升为 `PARTIAL_ISOLATED_SESSION_E2E`。这只覆盖直接用户的两项联系人媒体和可逆删除；回复/故事媒体、重启持久性、其它 peer 权限和原生频道媒体仍未验收。
- `go test ./app/bff/messages/internal/core ./app/messenger/msg/msg/internal/core -count=1` 通过；生产容器 `backend-backend-1` 仍为 `teamgram-server-latest:20261005-r12-prod`、`restart_count=0`、`oom=false`，本轮未重启数据库或其它依赖。
- 当前 813 行生产状态按 CSV 重新计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=291`、`PARTIAL_ISOLATED_SESSION_E2E=179`、`PARTIAL_COMPONENT_ONLY=156`、`NOT_ACCEPTED=110`、`PARTIAL_ISOLATED_DB_ONLY=48`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 生产直接消息按 ID 与反应复核（backend-backend-1）

- 聚焦探针在真实 DC2 会话中发送一条带唯一 tag 的私聊消息；`messages.getMessages` 通过 `InputMessageID` 返回 `messages.Messages`，`messages.readMessageContents` 返回 `messages.AffectedMessages`。
- Bob 对同一条消息设置并清除一个 thumbs-up 反应；`messages.sendReaction` 两次返回 `Updates`，`messages.getUnreadReactions` 返回 `messages.Messages`，`messages.readReactions` 返回 `messages.AffectedHistory`。探针随后分别删除发送方和接收方副本，生产 MySQL 精确 tag 回读为 `active=0`、`deleted=2`。
- 因此 `messages.getMessages`、`messages.readMessageContents`、`messages.sendReaction`、`messages.getUnreadReactions` 和 `messages.readReactions` 都有新的生产会话证据；反应分页、跨频道语义、跨客户端更新、重启持久性和混合 peer 原子性仍保持部分验收。
- 当前 813 行生产状态按 CSV 重新计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=291`、`PARTIAL_ISOLATED_SESSION_E2E=181`、`PARTIAL_COMPONENT_ONLY=156`、`NOT_ACCEPTED=108`、`PARTIAL_ISOLATED_DB_ONLY=48`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-05 r12 BFF/消息进程热替换复核（backend-backend-1）

- 当前工作树的支付请求校验、QR trusted-app/DC 原子认领和 canonical 频道读取改动已编译为 Linux ARM64：`bff` SHA-256 为 `693fe16945c217e40306d63d4acd5422335272db744564a23145e4494b30be90`，`msg` SHA-256 为 `3adaf88716956c5c20890987fc491e59a048cf0eb253f3bbad8e1985f9d3b30e`。旧二进制保留在 `/app/bin/bff.before-20261005-r12` 和 `/app/bin/msg.before-20261005-r12`。
- 先在 `backend-backend-1` 内停止并重启了 `msg`/`bff` 两个进程，随后用只包含 `backend` 的 Compose 覆盖文件重建为固定镜像 `teamgram-server-latest:20261005-r12-prod`（镜像 digest `sha256:b1ab0b5d6a7093ef106cc9914c148d8c363ff7cd1dd97c39ab7f8b37db73984a`）。当前容器为 `running`、`restart_count=0`、`oom=false`，其它 Teamgram 进程及 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn 和 identity 未重启。
- 新进程启动日志确认连接生产 MySQL、etcd、Redis/Kafka 依赖并完成服务注册；替换后窗口没有 `panic`、`runtime error`、fatal、500、超时或未预期错误。多媒体探针之前留下的唯一 tag 已按完整前缀清理，生产库 active 行数为 0。
- 聚焦回归通过：`go test ./app/bff/qrcode/... ./app/bff/authorization/... ./pkg/code/... -count=1`、`go test ./app/bff/passkey/... ./app/bff/messages/... ./app/bff/files/... ./app/service/authsession/... ./app/messenger/msg/inbox/... ./app/interface/session/... -count=1`、APIFull core/domain/qrcode `go test -c` 和 `git diff --check`。APIFull runtime 测试仍因本机 `127.0.0.1:13306/teamgram_audit` 隔离库不可达而未运行，未连接生产库。
- 当前台账仍为 813 个唯一方法，生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=291`；完整生产级 Layer 229 仍为 `0/813`。短信/邮件真实 provider、支付结算 provider、Passkey RP/认证器、QR trusted-app registry/第二 DC、原生频道媒体/存储、礼物库存结算和通话媒体控制面仍缺权威依赖，继续 fail-closed。

## 2026-10-05 r8 生产候选部署复核（backend-backend-1）

- 从当前工作树构建 `teamgram-server-latest:20261005-r8-prod`，通过 `/Users/mac/open/tg/services/backend/docker-compose.yaml` 加载 `docker/production/backend-deploy-r8-prod.yaml`，仅替换 `backend-backend-1`。目标容器当前为 `running`、`restart_count=0`，镜像为 `teamgram-server-latest:20261005-r8-prod`。
- 容器内 `idgen`、`status`、`authsession`、`dfs`、`media`、`biz`、`msg`、`sync`、`bff`、`session`、`gnetway` 均在运行；主机端口 `10443`、`11443`、`20010`、`20020`、`20030`、`20110`、`20120`、`20450`、`20640`、`20650`、`20660`、`20670` 均已监听，etcd 服务注册指向新容器地址。最近日志未发现 `panic`、`fatal`、连接拒绝或非预期 500。
- 本轮没有重启、迁移或写入 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn、identity 或其它服务；生产配置仍未提供真实短信/邮件、支付结算、Passkey RP/硬件认证器、QR trusted-app registry、第二 DC、原生频道媒体/存储、礼物库存结算或通话媒体控制面，这些方法继续按 fail-closed 处理。
- 当前 `LAYER229_METHOD_LEDGER.csv` 为 813 个唯一方法；生产列准确计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=177`、`PARTIAL_COMPONENT_ONLY=156`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`。完整生产级 Layer 229 仍为 `0/813`。
- 组件回归通过：`go test ./app/bff/authorization/internal/core ./app/bff/apifull/internal/persist ./app/bff/apifull/persist ./app/bff/passkey/... ./app/bff/qrcode/... ./app/bff/authorization/... ./pkg/code/... -count=1`、`git diff --check`。APIFull 全包未运行，因为当前环境未设置隔离 `teamgram_audit` DSN；没有误连生产库。当前 shell 也没有生产 GramJS 授权会话，因此没有新增真实用户 session 证据。

## 2026-10-05 r7 生产镜像状态复核（backend-backend-1）

- 当前目标容器运行 `teamgram-server-latest:20261005-r7-prod`，镜像 digest 为 `sha256:87ac0dce0f3e52e2418fdd99fdfa1dd874bb24a2c6383b32ae71dcbe6493fc5e`，容器状态为 `running`、`restart_count=0`。容器内 `idgen`、`status`、`authsession`、`dfs`、`media`、`biz`、`msg`、`sync`、`bff`、`session`、`gnetway` 均在运行。
- 本轮只复核已部署容器和日志，没有重启或迁移生产 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn 或其它服务。最近 30 分钟日志未出现 `panic`、`fatal`、500 或未预期的 `METHOD_NOT_IMPL`。
- 生产配置仍未提供真实短信/邮件、支付结算、Passkey RP/硬件认证器、QR trusted-app registry、第二 DC、原生频道媒体/存储、礼物库存结算或通话媒体控制面；这些功能继续 fail-closed，不能由 r7 的启动成功提升为生产级通过。当前 CSV 813 行计数为：生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=293`、`PARTIAL_ISOLATED_SESSION_E2E=177`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`、隔离组件 10、隔离库 8、传输包装 11。
- 当前 shell 没有 `TEAMGRAM_GRAMJS_DIR`、生产授权会话或用户 access hash，因此本轮没有重新执行 `probe-real-chain.ts`/`probe-reversible.ts`；本条只记录部署与日志复核，不把它记为新的生产 session 证据。组件回归仍以已通过的 `go test ./app/bff/passkey/... ./app/bff/qrcode/... ./app/bff/authorization/... ./pkg/code/... -count=1`、`go test ./app/service/authsession/... ./app/messenger/msg/inbox/... -count=1`、`go test ./app/bff/files/... ./app/bff/messages/internal/core ./app/interface/session/internal/sess ./app/interface/gnetway/internal/server/gnet -count=1` 和 `git diff --check` 为准；APIFull 全包需要显式隔离 `teamgram_audit` DSN，未误连生产库。

## 2026-10-05 密码重置等待状态原子更新

- `account.resetPassword` 的首次请求状态不再使用无条件 read/modify/write。共享 KV 增加 compare-and-swap：内存 store 用互斥锁，Redis store 用 Lua 原子脚本，APIFull MySQL store 用事务 `SELECT ... FOR UPDATE`；授权 handler 在竞争失败时重新读取并重试，保留最初的 `reset_until_date`/`reset_retry_date`。
- `go test ./app/bff/authorization/internal/core ./app/bff/apifull/internal/persist ./app/bff/apifull/persist -count=1` 与 `git diff --check` 通过。该修复只证明组件级并发边界，密码恢复完成、真实验证码/邮件 provider、重启持久性和生产 session 仍未验收。
- 台账从 `KNOWN_INCOMPLETE_STATIC/BLOCKED_BY_IMPLEMENTATION_GAP` 调整为 `PARTIAL_COMPONENT_VERIFIED/PARTIAL_COMPONENT_ONLY`；当前 813 行生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=177`、`PARTIAL_COMPONENT_ONLY=156`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`，另有隔离组件 10、隔离库 8、传输包装 11。

## 2026-10-05 生产 moderation intake 复核（backend-backend-1）

- 可逆探针增加了受环境变量保护的 `account.reportPeer` 分支。生产用户 Alice 对 Bob 发起带唯一消息文本的 `InputReportReasonOther` 举报，返回 `Bool`；通过生产 MySQL 观察到新的 `apifull_report` intake 行，随后按探针前最大 ID、actor、target 和 kind 精确删除，回读为 0。
- 这只证明举报请求已经走过 DC2 WebSocket → session → BFF → APIFull → 生产数据库并返回 typed response。仓库仍没有权威 moderation worker/provider 来审核、消费或执行封禁，因此 `account.reportPeer` 在台账中保持 `BLOCKED_BY_IMPLEMENTATION_GAP`，没有把 intake 入库当作完整实现。
- 本轮窗口日志出现少量 session `invalid salt`，session 按协议发送 `bad_server_salt`；未观察到对应的 500、panic、fatal 或业务 RPC 超时。该协议恢复事件单独保留为运行时风险，不计作举报功能成功证据。

## 2026-10-05 生产通知设备注册复核（backend-backend-1）

- 在同一可逆生产探针中为用户 `136907713` 注册唯一 `token_type=10` 临时设备 token，然后立即注销；真实返回均为 `Bool`，链路覆盖 DC2 WebSocket → gateway → session → BFF → notification service → 生产存储。
- 注销发生在同一探针的 `finally` 清理路径，未触碰用户既有设备记录；Redis 回读 `device:136907713:10:prod-notification-*` 为 0。本轮没有把外部 APNS/FCM/WebPush 投递当作已实现，也没有宣称重启持久化或推送送达。
- `account.registerDevice` 与 `account.unregisterDevice` 的生产证据已写入台账，生产状态仍保持 `PARTIAL_ISOLATED_SESSION_E2E`，因为通知 provider consumer 尚未接入。

## 2026-10-05 生产 `messages.getOutboxReadDate` 真实链路复核（backend-backend-1）

- 扩展 `docker/production/probe-reversible.ts`：使用生产用户 `136907713`/`136907714` 的现有授权会话，Alice 发送带唯一 tag 的临时私聊消息，Bob 通过 `messages.getHistory` 读到并调用 `messages.readHistory`，Alice 随后调用 `messages.getOutboxReadDate`。
- 真实返回类型为 `Updates`、`messages.Messages`、`messages.AffectedMessages` 和 `OutboxReadDate`，`OutboxReadDate.date` 为非零值；链路覆盖 DC2 WebSocket → gateway → session → BFF → message service → 生产 MySQL → typed response。生产库清理后该 tag 的 active message 为 0，保留 4 条删除 tombstone，符合现有消息审计清理约定。
- 聚焦测试 `go test ./app/bff/messages/internal/core -run 'TestMessagesGetOutboxReadDate' -count=1` 通过；探针只在单个本地进程环境中读取授权材料，没有打印或写入会话值。隐私拒绝、过期消息和未支持隐私规则仍按协议失败关闭，尚未提升为真实生产分支证据。
- `messages.getOutboxReadDate` 已从 `KNOWN_INCOMPLETE_STATIC/BLOCKED_BY_IMPLEMENTATION_GAP` 提升为 `PARTIAL_SESSION_VERIFIED/PARTIAL_ISOLATED_SESSION_E2E`。当前台账为 `BLOCKED_BY_IMPLEMENTATION_GAP=293`、`PARTIAL_ISOLATED_SESSION_E2E=177`；完整 Layer 229 仍未达到 813/813。

## 2026-10-04 r6 生产部署与 CDN 失败关闭复核（backend-backend-1）

- 移除了不安全的 CDN token 路径：此前实现从公开的 auth key ID 派生 HMAC 密钥，任何知道 ID 的调用方都可以伪造 token，不能作为生产实现。已删除 `app/bff/files/internal/core/cdn_token.go` 及其三条伪成功集成；`upload.getFile`、分片上传和文件哈希仍保留真实输入校验与 DFS 路径，缺少权威 CDN provider 的 `upload.getCdnFile`、`upload.getCdnFileHashes`、`upload.reuploadCdnFile` 继续失败关闭。工作树确认没有 `decodeCDNToken` 或 `cdn_token` 引用。
- `go test ./app/bff/files/... ./app/service/authsession/... ./app/interface/session/... -count=1` 以及部署后 `go test ./app/bff/authorization/... ./pkg/code/... ./app/bff/passkey/... ./app/bff/qrcode/... ./app/bff/files/... -count=1` 均通过，`gofmt` 与 `git diff --check` 通过。生产镜像使用 `Dockerfile.production-r5` 在已验收的 `teamgram-server-latest:20261004-r4` runtime 之上重建，镜像为 `teamgram-server-latest:20261004-r6-prod`（image digest `sha256:a7627d1b1ed86f2892ef8800e5e92ef29a5ce53e0d4d92df9e7dc0588228c157`）。
- 通过 backend Compose 仅强制重建 `backend-backend-1`，没有重启或迁移 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn 或其它容器。新容器为 `running`、`restart_count=0`；idgen/status/authsession/dfs/media/biz/msg/sync/bff/session/gnetway 十一个进程均已启动。部署后的关键二进制 SHA-256 为 `bff=dd3633bbe30a9173ebfbee5edadc467b90166e7b6b39fc4cceff5686afa916a6`、`authsession=1f3002db6aee1ff8179d35d6442b5d98981145f53118c4d3b4c885fac933ab7a`、`msg=99052bd90ef4b5e096f137e3d042463bad3ab75997cf14fc9a85c2d8d92046c5`、`biz=7f45a0e95c41775e99770ee10055af6ce0716155127ab77039c6fe96b6fef03c`、`session=bba82421a4664e3d6a75e70d77fde658e15ebc00ee8edd4b926bcc069c1062cd`、`gnetway=e028cfa7b9a1ba7aa80aff6d9ba1b0bc7027e958a9fd86ae2438a32045fb8544`。
- 容器启动后日志只有常规连接数和资源统计，未见 `panic`、`fatal`、500 或启动失败；生产配置继续为空的 Passkey RP/QR 注册表并保持 fail-closed。该部署只证明新二进制可启动，不提升短信/邮件真实 provider、支付结算、Passkey 硬件认证器、第二 DC、原生旧频道存储、礼物结算或通话媒体控制面的生产验收状态。
- 当前 813 行台账计数仍为：生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=294`、`PARTIAL_ISOLATED_SESSION_E2E=176`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、`TRANSPORT_ONLY=11`；完整生产级 Layer 229 仍为 **0/813**。

## 2026-10-04 r4 生产 BFF 部署与探针残留清理（backend-backend-1）

- 当前工作树通过聚焦包回归：`go test ./app/bff/files/internal/core ./app/bff/users/internal/core ./app/bff/messages/internal/core ./app/bff/contacts/internal/core ./app/bff/dialogs/internal/core ./app/bff/authorization/internal/core ./app/bff/passkey/internal/core ./app/bff/qrcode/internal/core ./app/service/authsession/internal/dao ./pkg/code/... -count=1`；`go build ./app/bff/...` 与 `git diff --check` 通过。APIFull 全包仍要求显式隔离 `127.0.0.1:13306/teamgram_audit` DSN，本轮未让测试误连生产库。
- `upload.getFileHashes` 已从“只有路由”改为组件已验证：使用真实 DFS client 按 128 KiB 分块读取并返回带 offset/limit/SHA-256 的 `FileHash`，focused fake-DFS 测试覆盖多分块、空位置和负 offset。生产列仍保持 `PARTIAL_COMPONENT_ONLY`，没有把 fake-DFS 证据冒充生产 session 验收。
- 因 Docker Hub 拉取 `ubuntu:latest` 临时 EOF，未重建基础运行时；从已验收的 r3 运行时制作 `teamgram-server-latest:20261004-r4`，只替换当前工作树编译的 Linux ARM64 BFF（SHA-256 `eaaeb4de8a1d239df945593a9ee84f571c96a546caf6ab1fa6d0437b5d4050c4`），再通过 backend Compose 仅重建 `backend-backend-1`。容器现为 `running`、`restart_count=0`，其余业务和基础设施没有重启或迁移。
- 生产 Compose 额外挂载 `docker/production/bff.production.yaml`，移除隔离环境的 Passkey/QR fixture allowlist；BFF 启动日志回读 `Passkey.RelyingPartyId` 和 `QrCode.TrustedApps` 为空，生产请求会在缺少真实 RP/应用注册表时 fail-closed。隔离栈仍使用原 `teamgramd/etc2/bff.yaml` fixture 配置。
- 重建后日志完成 idgen/status/authsession/dfs/media/biz/msg/sync/bff/session/gnetway 启动和 etcd 注册检查，窗口内无 `panic`、`fatal`、500 或端口冲突。生产 MySQL 精确回读确认 `prod-chain-*` 活跃消息、`prod-call-*` 通话和 tagged `apifull_kv` 均为 0；本轮只清理了被中断探针生成的 tagged 行和临时群，消息 tombstone 保留。
- 生产级 Layer 229 仍不是 813/813。当前台账仍有短信/邮件 provider、支付/退款/Stars 结算、Passkey RP/硬件认证器、第二 DC、原生旧频道存储、礼物库存结算、通话媒体 relay/signaling/RTMP，以及未覆盖的业务方法阻塞；本次部署没有改变这些依赖边界。
- 复核 r4 后清理了生产库中本轮探针遗留的 7 条近期 `discarded` secret chat 和 10 条对应 device-key：限定双方用户 `136907713`/`136907714`、近 6 小时、无 `apifull_secret_message` 后执行事务删除，删除前后回读分别为 7/10 与 0/0；没有触碰活跃聊天、历史消息或用户级 QTS。
- 台账当前为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=286`、`PARTIAL_SESSION_VERIFIED=167`、`PARTIAL_COMPONENT_VERIFIED=163`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_VERIFIED=55`；生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=294`、`PARTIAL_ISOLATED_SESSION_E2E=176`、`PARTIAL_COMPONENT_ONLY=155`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`，其余为隔离/传输包装项。完整生产级 Layer 229 仍为 **0/813**。

## 2026-10-04 r3 生产秘密聊天跨设备密钥验收（backend-backend-1）

- 生产探针 `docker/production/probe-secret-chat-dh.ts` 现在要求同一用户的两个永久 auth key。使用生产用户 `136907713` 与 `136907714` 的现有授权会话，通过 DC2 WebSocket 完成 `messages.getDhConfig`、`messages.requestEncryption`、`messages.acceptEncryption`、`messages.sendEncrypted`、`updates.getDifference`、`messages.readEncryptedHistory` 和 `messages.discardEncryption`。
- DH 配置返回 `messages.DhConfig`（version 1、256 字节质数、generator 3），两端计算出的共享密钥和 fingerprint 一致。用户 `136907714` 的两个设备用不同的 `g_b`/fingerprint 接受同一 secret chat，均返回 `EncryptedChat`；生产 MySQL 对该临时 chat 持久化了发起设备和两个接收设备的 3 条 `apifull_secret_chat_device_key` 记录，旧设备重试没有覆盖第二设备的新 canonical key。
- 第二设备发送的随机密文返回 `messages.SentEncryptedMessage`；Alice 在发送前读取当前 QTS=1，随后用公开 `updates.getDifference` 收到 typed `updates.Difference`，QTS 前进到 2，密文字节逐字节匹配。重连后再次发送和读取历史也返回正确类型。探针以 `delete_history=true` 丢弃临时 chat，生产库没有留下该 chat 的 active 行或 secret message；用户 QTS 只保留单调游标，没有回退或重置。
- 本轮没有重启 `backend-backend-1`、数据库或其它服务。探针结束后按 chat ID、双方用户和 discarded 状态精确删除了本轮 chat 及其 3 条 device-key 行；用户级 QTS 和其它 secret-chat 行没有回退或盲删。GramJS `destroy()` 清理阶段的 `Not connected`/WebSocket close 不影响上述 RPC 结果。
- 因此 `messages.getDhConfig`、`messages.requestEncryption`、`messages.acceptEncryption`、`messages.discardEncryption`、`messages.readEncryptedHistory`、`messages.sendEncrypted` 和 `updates.getDifference` 的生产列提升为 `PARTIAL_ISOLATED_SESSION_E2E`。`messages.sendEncryptedService`、`messages.receivedQueue`、`messages.sendEncryptedFile` 和 `messages.reportEncryptedSpam` 仍缺少本轮对应的真实 provider/流程证据；完整 Layer 229 仍不是 813/813。

## 2026-10-04 r3 生产真实链路复核补充（backend-backend-1）

- 修正生产探针的连接目标：`probe-real-chain.ts` 和 `probe-control-plane.ts` 现在像可逆探针一样在 `CallbackSession.load()` 后注入 `TG_DC_HOST`/`TG_DC_PORT`，默认连接 `127.0.0.1:11443`，不会误连 GramJS 默认的 443。
- 使用生产库现有用户 `136907713` / `136907714` 的授权会话重新执行全链路探针。`probe-real-chain.ts` 返回并校验 `User`、`Config`、`NearestDc`、`help.CountriesList`、`help.TimezonesList`、`messages.AvailableReactions`、`messages.SavedGifs`、`messages.DialogsSlice`、basic group 的 `messages.ForumTopics`/`GetForumTopicsByID`、直接消息搜索/全局搜索/已读/编辑、反应列表、公开投票、定时消息和 canonical APIFull 频道差分/阅读数/编辑/删除；删除后的按 ID 读取得到预期 `MESSAGE_ID_INVALID`。临时数据清理后，生产库 `messages(message LIKE 'prod-chain-%', deleted=0)=0`、`apifull_group_call(title LIKE 'prod-call-%')=0`。
- 复核发现历史探针留下的 3 条 KV 缓存行（投票未读、定时消息和已销毁通话的 tagged 记录）；按 `prod-chain-*`/`prod-call-*` 精确过滤数组并删除已销毁通话记录后，生产库 tagged KV、active messages 和 active group calls 均为 0，用户其它 KV（包括现有 quick-reply 项）保留。
- `probe-control-plane.ts` 首次调用因探针未提供成员 access hash 得到 `USER_ID_INVALID`；补充从生产用户表读取的真实 access hash 后重新通过 `phone.createGroupCall`、`phone.editGroupCallTitle`、`phone.getGroupCall`、`phone.inviteToGroupCall`、`phone.joinGroupCall`、`phone.leaveGroupCall`、`phone.discardGroupCall`，返回 `Updates`/`phone.GroupCall`。非成员改标题的 `CHAT_ADMIN_REQUIRED` 和销毁后读取的 `GROUPCALL_INVALID` 是预期授权/生命周期校验。
- 最新 CSV 仍为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=286`、`PARTIAL_COMPONENT_VERIFIED=162`、`PARTIAL_SESSION_VERIFIED=166`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=113`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=294`、`PARTIAL_ISOLATED_SESSION_E2E=176`、`PARTIAL_COMPONENT_ONLY=154`、`NOT_ACCEPTED=112`、`PARTIAL_ISOLATED_DB_ONLY=48`，其余为隔离组件/数据库、传输包装或 session 手动分发。生产容器仍为 `teamgram-server-latest:20261004-r3`，`running` 且 `restart_count=0`；本轮没有重启数据库或其它服务。重试后的服务日志没有 500、panic 或业务超时；首次探针配置错误只产生了预期的 400 输入校验。通话媒体 relay、第二 DC、短信/邮件真实 provider、支付结算、Passkey RP/硬件认证器、原生旧频道存储、礼物库存结算和其它未覆盖的秘密聊天操作仍未达到生产级验收，完整 Layer 229 继续为 **0/813**。

## 2026-10-04 r3 生产 BFF 更新与回归（backend-backend-1）

- 在 r2 运行时层之上构建 `teamgram-server-latest:20261004-r3`（镜像 `sha256:ed51830b8b81293afdba18d2594f817cc69642ffaa5301556a234375b10509c8`），只更新当前工作树编译的 Linux ARM64 BFF；`authsession`、`msg`、`biz` 和其它基础设施沿用已验收二进制。通过 backend Compose 仅重建 `backend-backend-1`，生产 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn、其它容器均未重启或迁移。
- 新容器状态为 `running`、`restart_count=0`，BFF SHA-256 为 `b7f9f344a9a1f63062f6439c7530cfe768f66038ef49053106676c6f6c0698ed`；其余关键二进制保持 `authsession=bdeb361f788e6c471cc6c7270d0079ceb0ad933c5627c56b610aab27b0a88316`、`msg=8e1b6df1eb695d839fad55a0457afea5222baf4d4cc36e0eb48fba7571efbc3d`、`biz=679521256ac0a258de9e472dcd79919223510a0773af1df9349a002c7c9d8531`。
- 部署后使用生产用户 `136907713` / `136907714` 的现有授权会话，通过 DC2 WebSocket `127.0.0.1:11443` 重跑可逆探针：`account.get/setAccountTTL`、`account.get/updateNotifySettings`、`contacts.add/get/deleteContacts`、`messages.saveDraft/getPeerDialogs`、`messages.toggleDialogPin/getPinnedDialogs` 均返回正确类型并读回，随后恢复原值。生产库未留下 active 草稿、联系人或探针消息；已有通知设置和删除 tombstone 与探针前状态一致。
- 当前 813 行台账计数为：`KNOWN_INCOMPLETE_STATIC=287`、`PARTIAL_COMPONENT_VERIFIED=160`、`PARTIAL_SESSION_VERIFIED=158`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=122`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、隔离组件 10、隔离库 8、传输包装 11、session 手动分发 2；生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=295`、`PARTIAL_ISOLATED_SESSION_E2E=161`、`PARTIAL_COMPONENT_ONLY=157`、`NOT_ACCEPTED=122`、`PARTIAL_ISOLATED_DB_ONLY=49`、隔离组件 10、隔离库 8、传输包装 11。
- 本轮没有把 provider 缺失的方法标成成功；短信/邮件、支付/退款/Stars、Passkey 真实认证器、第二 DC、原生旧频道存储、礼物库存结算和通话媒体面仍是生产级阻塞，完整 Layer 229 仍为 `0/813`。

## 2026-10-04 r2 继续：隔离 APIFull 全包回归

- 使用隔离 `teamgram_audit` 数据库（仅通过临时端口转发访问，未连接生产 `teamgram` 库）执行 `APIFULL_MYSQL_DSN='...@tcp(127.0.0.1:13306)/teamgram_audit?charset=utf8mb4&parseTime=true' go test ./app/bff/apifull/... -count=1`，`channelview`、`internal/core`、`internal/domain` 和 `layer229` 全部通过。
- 本轮只修正测试夹具与断言：频道删除后的 pts 期望从旧值 5/7/8 调整为当前持久化序列 6/8/9；支付回读使用每次测试唯一 transaction ID；空请求在 provider 前返回 `INPUT_REQUEST_INVALID`/字段校验错误；测试对 gRPC status 包装和 typed-nil 结果按协议判断。没有放宽生产参数校验，也没有把缺 provider 的方法改成成功返回。
- 同步回归通过：`go test ./app/bff/passkey/... ./app/bff/authorization/... ./app/bff/qrcode/... ./app/service/authsession/... ./app/bff/files/... ./app/bff/messages/... -count=1`。生产容器未重启，`backend-backend-1` 仍为 `teamgram-server-latest:20261004-r2`。
- 当前 CSV 仍为 813 个唯一方法；生产验收状态为 `BLOCKED_BY_IMPLEMENTATION_GAP=295`、`PARTIAL_ISOLATED_SESSION_E2E=161`、`PARTIAL_COMPONENT_ONLY=154`、`NOT_ACCEPTED=125`、`PARTIAL_ISOLATED_DB_ONLY=49`、隔离组件 10、隔离库 8、传输包装 11。完整生产级 Layer 229 仍为 **0/813**，因为短信/邮件真实 provider、外部支付结算、Passkey RP/硬件认证器、第二 DC、原生频道完整存储、礼物库存结算和通话媒体面仍没有权威后端或生产证据。

## 2026-10-04 r2 生产目标重新验收（backend-backend-1）

- `backend-backend-1` 已运行 `teamgram-server-latest:20261004-r2`，容器状态为 `running`、`restart_count=0`；本轮只重建该业务容器，没有重启或修改生产 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn 或其它基础设施。容器内关键二进制 SHA-256 为：`bff=301cf2d747830d9ed7db07865e17ef0b53d524a6769e23cb060bdbb9d75015db`、`authsession=bdeb361f788e6c471cc6c7270d0079ceb0ad933c5627c56b610aab27b0a88316`、`msg=8e1b6df1eb695d839fad55a0457afea5222baf4d4cc36e0eb48fba7571efbc3d`、`biz=679521256ac0a258de9e472dcd79919223510a0773af1df9349a002c7c9d8531`。
- 使用生产库现有用户 `136907713` / `136907714` 的授权会话，在 DC2 WebSocket `127.0.0.1:11443` 重新执行 `probe-real-chain.ts`、`probe-control-plane.ts` 和 `probe-reversible.ts`。授权密钥只在本地探针进程环境变量中读取，未写入日志、镜像或台账。
- `probe-real-chain.ts` 返回并校验了真实类型：直接消息发送/历史/搜索/全局搜索/已读/编辑、反应列表、basic group 创建/发送/历史/`messages.ChatFull`/改标题、公开投票双方投票/结果/投票者、定时消息创建/读取/取消/强制投递、canonical APIFull 频道创建/邀请/发送/历史/搜索/按 ID 读取/已读/阅读数/频道差分/编辑/删除。频道阅读数为 `messages.MessageViews`，差分为 `updates.ChannelDifference`，删除后按 ID 读取得到 `MESSAGE_ID_INVALID`。
- `probe-control-plane.ts` 重新验证了 `phone.createGroupCall`、`phone.editGroupCallTitle`、`phone.getGroupCall`、`phone.inviteToGroupCall`、`phone.joinGroupCall`、`phone.leaveGroupCall`、`phone.discardGroupCall`，返回 `Updates`/`phone.GroupCall`；`probe-reversible.ts` 重新验证了 TTL、直接用户通知设置、联系人增删、草稿保存/清除和对话置顶/取消置顶，均完成生产 MySQL 读回并恢复原值。
- 生产库回读确认本轮及历史同类探针没有活跃残留：`messages(message LIKE 'prod-chain-%', deleted=0)=0`、`apifull_channel(title LIKE 'prod-chain-%')=0`、`apifull_channel_message(message LIKE 'prod-chain-%')=0`、`chats(title LIKE 'prod-chain-%', deactivated=0)=0`、`apifull_group_call(title LIKE 'prod-call-%')=0`、定时消息 `state=0` 为 0。删除的 basic group 仅保留 `deactivated=1` tombstone，符合可审计清理约定。
- 三支探针在输出完整业务结果后，GramJS `destroy()` 的后台发送循环打印了 `Not connected`/WebSocket close；这些发生在清理阶段，不是业务 RPC 返回错误。生产服务日志在验收窗口未出现 500、panic、fatal 或超时。媒体 relay、第二 DC、短信/邮件真实 provider、支付结算、Passkey RP、原生旧频道存储、秘密聊天跨设备密钥、礼物库存结算和通话媒体控制面仍未达到生产级验收，因此完整 Layer 229 仍为 **0/813**。
- 验证命令：`go test ./app/bff/passkey/... -count=1`、`go test ./app/bff/authorization/... -count=1`、`go test ./app/bff/qrcode/... ./app/service/authsession/... -count=1`、文件/消息相关 BFF 包测试、隔离 DSN 下的 `go test ./app/bff/apifull/... -count=1` 均通过；`go build ./app/bff/...` 与 `git diff --check` 通过。APIFull 全包通过只说明隔离实现和协议断言一致，不提升生产级完整验收状态。

## 2026-10-04 本轮继续：生产频道编辑/删除与通话控制面复核

- 当时生产目标 `backend-backend-1` 运行 `teamgram-server-latest:20261004-r1`；本轮没有重启容器、数据库、Redis、Kafka、etcd、SeaweedFS 或其它基础设施。现有生产授权会话只在探针进程内读取，未写入日志或台账。
- 复跑真实 DC2 WebSocket 探针，使用用户 `136907713` / `136907714` 新建 APIFull canonical 频道并邀请成员。直接消息和频道消息均通过 `send → history/search/read → edit → delete`，频道 `getMessages` 在删除后返回 `MESSAGE_ID_INVALID`；`messages.getMessagesViews` 返回 `messages.MessageViews`，`updates.getChannelDifference` 返回带消息的 `updates.ChannelDifference`。临时消息、频道和频道消息已回读确认清理为 0；旧 `channels/channel_participants/channel_messages` 表没有迁移或新增。
- 新增生产通话控制面探针：临时 `phone.createGroupCall`、`phone.editGroupCallTitle`、`phone.inviteToGroupCall`、`phone.getGroupCall`、`phone.joinGroupCall`、`phone.leaveGroupCall`、`phone.discardGroupCall` 全部通过真实 session → BFF → APIFull → MySQL，返回类型为 `Updates`/`phone.GroupCall`，丢弃后按标题查询 `apifull_group_call` 为 0。该证据只覆盖控制面；媒体 relay、signaling、RTMP、Stars 和跨重启回放仍未实现或未验收。
- APIFull 频道解析现在在 canonical MySQL 已配置时严格 fail-closed：缺少 `apifull_channel` 行直接 `CHANNEL_INVALID`，不会从旧 KV 记录复活；频道列表和 chatlist 同样过滤缺行。隔离 MySQL 的 domain/channel/channelview/folder focused tests 通过；既有 `TestChannelDeleteMethods` 的 pts 断言仍因 pin 后实际 pts 递增到 6 而失败，该失败与本次收紧无关。
- Passkey 继续加强配置和句柄边界：要求有效本地 `DcId`、非空 RP display name、canonical `dc_id:user_id` userHandle、`id` 与 `raw_id` 均为相同非空 raw-url base64；缺失或跨 DC 句柄失败关闭。`go test ./app/bff/passkey/... -count=1` 通过，隔离 options ceremony 通过；真实浏览器/硬件认证器、生产 RP/provider 和第二 DC 仍未验收。
- 当前 CSV 为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=288`、`PARTIAL_COMPONENT_VERIFIED=155`、`PARTIAL_SESSION_VERIFIED=158`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=126`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、`ISOLATED_COMPONENT_VERIFIED=10`、`ISOLATED_DB_VERIFIED=8`、传输包装 11、session 手动分发 2。生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=296`、`PARTIAL_ISOLATED_SESSION_E2E=161`、`PARTIAL_COMPONENT_ONLY=152`、`NOT_ACCEPTED=126`、`PARTIAL_ISOLATED_DB_ONLY=49`、隔离组件 10、隔离库 8、传输包装 11；完整生产级 Layer 229 验收仍为 **0/813**。

## 2026-10-04 本轮继续：生产 basic-group 真实链路复核

- 同一生产 DC2 WebSocket 探针使用用户 `136907713` / `136907714` 新建临时 basic group。`messages.createChat` 返回 `messages.InvitedUsers`，随后由 `messages.getDialogs` 取得新群实体；群消息通过 `messages.sendMessage` 写入，成员用 `messages.getHistory` 和 `messages.getFullChat` 读回，`messages.editChatTitle`/`messages.getChats` 验证标题持久化。
- `messages.deleteChat` 返回 `Boolean`，生产 MySQL 对应 `chats` 行 `deactivated=1`，临时消息全部为 deleted tombstone、无 active probe message。该流程未修改旧频道表、没有清库或迁移。

## 2026-10-04 本轮继续：生产定时消息真实链路复核

- 同一生产探针通过 `messages.sendMessage(schedule_date)` 创建未来消息，使用 `messages.getScheduledHistory` / `messages.getScheduledMessages` 读回；第二条由 `messages.deleteScheduledMessages` 取消，第三条由 `messages.sendScheduledMessages` 强制投递，并由另一真实会话的 `messages.getHistory` 读回。
- 探针清理了已投递消息、取消剩余定时项、临时群和频道；生产 MySQL 回读显示该 tag 没有 active 消息，basic group 为 `deactivated=1`，canonical 频道及频道消息均为 0。

## 2026-10-04 本轮继续：生产账户、联系人和对话状态复核

- 可逆探针使用同一对生产会话验证 `account.getAccountTTL`/`setAccountTTL`（180 → 30 → 180）、`account.getNotifySettings`/`updateNotifySettings`（直接用户静音/预览设置写入、读回、恢复）、`contacts.addContact`/`getContacts`/`deleteContacts`（临时联系人写入、读回、删除）、`messages.saveDraft`/`getPeerDialogs`（草稿写入、读回、清除）以及 `messages.toggleDialogPin`/`getPinnedDialogs`（置顶、读回、取消）。
- 所有操作都走真实 WebSocket → gateway → session → BFF → User/Contacts/Dialog 服务 → 生产 MySQL，探针结束恢复原值；没有修改既有联系人、草稿或置顶状态，也没有留下 active 临时数据。

## 2026-10-04 本轮继续：生产投票真实链路复核

- 生产探针在临时 basic group 发送公开投票，两个真实会话都读到同一服务端 poll ID/hash；双方分别投票后，`messages.getPollResults` 返回 `Updates`，`messages.getPollVotes` 返回两名真实投票者，投票消息视图也可读回。
- 探针随后删除临时群；生产 MySQL 回读显示该 tag 没有 active 消息。投票统计的全量 analytics、匿名投票、重启恢复和跨群分页仍未验收。

## 2026-10-04 生产 canonical 频道真实链路复核（backend-backend-1）

- 当时目标容器 `backend-backend-1` 使用 `teamgram-server-latest:20261004-r1`，`restart_count=0`；本轮没有重启数据库、Redis、Kafka、etcd、SeaweedFS 或其它服务。BFF/Biz/Msg/Session 二进制已在容器内读取并记录 SHA-256，生产探针通过 DC2 WebSocket 连接。
- 使用生产库现有用户 `136907713` / `136907714` 的授权会话，由用户 B 新建 APIFull canonical 频道并邀请用户 A；不读取、不迁移旧 `channels/channel_participants/channel_messages` 数据。真实调用链 `WebSocket → session → BFF → APIFull → production MySQL` 完成：`channels.createChannel`、`channels.inviteToChannel`、`messages.sendMessage`、`messages.getHistory`、`messages.search`、`channels.getMessages`、`channels.readHistory`、`channels.readMessageContents`、`messages.getMessagesViews`、`updates.getChannelDifference`、`channels.deleteChannel`。
- 所有返回均为正确的 typed 对象：`Updates`、`messages.Message(s)`、`messages.MessageViews`、`updates.ChannelDifference`、`Boolean`；频道差分包含本次新消息，阅读数返回数值字段。发送、搜索、按 ID 读取、已读和差分均使用同一 `apifull_*` canonical 行。
- 探针最后删除临时消息/频道。生产数据库回读：`prod-chain-*` 活跃消息 0、活跃 `apifull_channel` 0、活跃频道消息 0、旧 `channels` 表同前无新增记录。无 500、超时、panic 或连接中断导致的业务失败；探针结束时 GramJS 销毁连接产生的 `Not connected` 为清理阶段噪声。
- 先前用旧频道 ID 直接发送得到的 `CHANNEL_INVALID` 是预期的 canonical access-hash/数据源边界：当前版本按约定不兼容旧表，也不做旧数据迁移。新业务必须通过 `channels.createChannel` 或已存在的 `apifull_channel` canonical 频道进入消息链路；旧表记录不计作当前实现缺口。
- 这次只证明 canonical 频道的生产 session 链路；频道媒体、跨重启事件回放、第二 DC、短信/邮件 provider、支付结算、Passkey RP、礼物库存和通话媒体控制面仍未完成生产验收。

## 2026-10-03 频道反垃圾设置 canonical 持久化

- `channels.toggleAntiSpam` 已从调用者 KV 请求记录改为校验频道 access hash/所有者后写入 canonical `apifull_channel.antispam`，返回带频道实体的 typed Updates；`channels.getFullChannel` 的 `ChatFull.antispam` 会读回同一字段。
- `TestChannelSettingsRoundTrip` 在隔离 `teamgram_audit` 覆盖设置写入、读回、误报 intake 及 Layer 229 编码；反垃圾审核/消息拦截消费者、委托管理员语义、原生频道 provider 和生产流量仍未验收。

## 2026-10-03 频道无活动列表 canonical DB 路径

- `channels.getInactiveChannels` 已从固定 `METHOD_NOT_IMPL` 改为查询 APIFull 的 canonical `apifull_channel` 与 `apifull_channel_message`：只返回当前用户创建且最后消息（无消息时取创建时间）超过 30 天未更新的频道，并按最后活动时间稳定排序。
- 返回值始终包含可编码的 `dates`、`chats`、`users` 向量；隔离数据库测试覆盖无消息频道、近期消息排除、时间戳读回和 Layer 229 编码。
- 这项仍是隔离数据库部分验收：未覆盖委托管理员语义、原生频道 provider、生产流量、重启后 session 回读。

## 2026-10-03 密码邮箱 provider 接线

- APIFull 的 `account.resendPasswordEmail` 和 `account.confirmPasswordEmail` 已接入共享 Redis challenge 与 `Code.EmailProvider`。重发只在 provider、Redis 和限流检查成功后保存一次性 challenge ID；确认使用原子消费，成功后清除 pending 状态；取消会撤销未消费 challenge。
- `bff` 将 `Code` 配置传入 APIFull，空配置仍失败关闭。新增路径只完成组件接线，未连接真实邮件 endpoint/Redis，也未改生产数据库或重启 `backend-backend-1`。

## 2026-10-03 短信缺码报告 provider 接线

- `auth.reportMissingCode` 现在使用独立的 `Code.ReportMissingCodeUrl` HTTP provider；请求包含规范化手机号、`phone_code_hash` 和用途，只有 provider 返回 2xx 才返回 `BoolTrue`。未配置 endpoint 或 provider 失败时继续失败关闭，不伪造已接收报告。
- 新增 provider 和 authorization core 测试覆盖显式 endpoint、JSON 负载、未配置拒绝与 provider 错误。当前仅完成组件接线，没有真实短信事故接收 endpoint、送达平台、隔离 session 或生产验收。

## 2026-10-03 短信/邮箱台账校正

- Passport 的 `account.sendVerifyPhoneCode` / `account.verifyPhone` 已按当前代码记录为共享 Redis challenge、短信 provider、一次性消费和 Passport 文档持久化的组件实现；密码邮箱确认/重发同样记录为共享 email challenge/provider 的组件实现。
- 这些条目仍没有真实短信/邮件 endpoint、完整 live session、重启持久性或生产验收，因此只提升 `audit_status`，不提升 `production_acceptance_status`。
- `account.cancelPasswordEmail` 的台账也已与实现同步：撤销未消费 challenge、清除邮箱待确认状态并持久化；新增回归测试，生产验收仍保持未通过。
- `help.getPassportConfig` 的台账已与现有 PassportCore 对齐：返回可编码的 typed config 和空国家 JSON；空列表保留为允许的表单例外，未虚报国家 provider 或生产验收。

## 2026-10-03 临时授权密钥输入门禁

- `auth.bindTempAuthKey` 在 BFF 层现在拒绝空请求、短密文、非块对齐密文和缺失 authsession provider，再把合法帧交给 authsession 的永久/临时密钥事务绑定。定向输入测试和 authorization 包测试通过；真实临时密钥握手仍未做 session→authsession 端到端验收。

## 2026-10-03 反应举报接入持久化接收队列

- `messages.reportReaction` 不再只返回 `METHOD_NOT_IMPL` 或写入无人消费的进程 KV；在 APIFull 数据库可用时，它校验消息 ID、目标 peer 和反应方 peer，并通过 `apifull_report` 的幂等接收表持久化请求。
- 没有把这项提升为已实现：反应举报的审核/处置消费者仍不存在，数据库不可用时继续失败关闭；本轮只做 `go test -c ./app/bff/apifull/internal/core`、`gofmt` 和 `git diff --check`，未连接生产库或隔离 MySQL。
- 同一接收路径还覆盖 `channels.reportSpam`、`messages.reportReadMetrics`、`messages.reportMusicListen` 和 `stories.report`；台账将这四项从静态缺口调整为组件部分实现，生产验收仍未通过。
- 当前 CSV 仍为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=289`、`PARTIAL_COMPONENT_VERIFIED=154`、`PARTIAL_SESSION_VERIFIED=155`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=128`、`PARTIAL_ISOLATED_DB_VERIFIED=56`，其余为隔离组件/数据库、传输包装或会话分发待验收项；生产级完整验收仍为 `0/813`。

## 2026-10-03 Firebase 短信挑战路径复核

- `auth.requestFirebaseSms` 当前已复用共享 SMS challenge/provider：Firebase token 先完成手机号绑定校验，短信挑战在 provider 成功后才更新 phone-code 状态，后续写入失败会撤销已签发挑战。
- 台账从静态缺口调整为组件部分实现；真实 Firebase/SMS 凭据、provider 送达、完整登录会话和生产验收仍未完成。当前 CSV 计数相应为 `KNOWN_INCOMPLETE_STATIC=315`、`PARTIAL_COMPONENT_VERIFIED=136`。

## 2026-10-03 资料照片链路失败路径复核

- `photos.getUserPhotos` 对用户服务/媒体服务错误和空响应失败关闭；`photos.uploadProfilePhoto` 校验上传结果与用户服务返回的照片 ID，照片更新/删除的同步投递错误不再被忽略。用户资料照片写入、删除及主照片切换在用户行锁定的事务中完成，并传播存储错误。
- 新增读取正常/错误路径、更新和删除同步错误的 BFF 单测，以及资料照片 DAO 审计库往返用例。三个相关包的完整 Go 测试通过，`git diff --check` 通过。
- `TestProfilePhotoMutationsAuditDatabase` 因未配置 `APIFULL_MYSQL_DSN` 跳过；它只接受 `127.0.0.1:13306/teamgram_audit`。本轮没有连接生产数据库、部署或重启服务，因此资料照片数据库往返和生产级端到端仍未验收。

## 2026-10-02 继续复核：生产只读探针与频道消息作者

- 生产目标 `backend-backend-1` 当前容器仍为 `teamgram-server-latest:20261002-r12`，`restart_count=0`；本轮没有重启容器、数据库、Redis、Kafka、etcd、DFS、msg 或其它 Teamgram 进程。`/app/bin/bff` SHA-256 仍为 `8fd3be5eed72b7f87a3491f07b18ee142e0d99a383a604e3f54c0cf2949b3b44`。
- 通过 `ws://127.0.0.1:11443/apiws` 的真实 MTProto WebSocket 只读探针返回：`help.getConfig → Config`、`help.getNearestDc → NearestDc`、`help.getCountriesList → help.CountriesList`、`help.getAppConfig → help.AppConfig`。第一次探针因客户端把 `hash` 编码为 BigInt 而失败，修正为 Layer 229 的 `int` 后四项均完成；服务端日志没有 500、panic、fatal 或超时。
- 使用生产库中用户 `136907714` 的现有授权密钥完成只读频道回读：`messages.getHistory → messages.Messages`（7 条）、`channels.getFullChannel → messages.ChatFull`、`channels.getParticipants → channels.ChannelParticipants`（1 名成员）、`messages.getMessagesViews → messages.MessageViews`、`updates.getChannelDifference → updates.ChannelDifference`（7 条新消息）。当前 Web GramJS 运行时没有导出 `channels.GetMessageAuthor` 构造器，因此该单项无法从客户端编码；这是客户端 schema 缺口，不能把它记成后端错误。
- 同一授权会话调用 `messages.getSavedHistory(peer=self)` 返回 `messages.Messages`（0 条消息、1 个用户实体）。这是当前生产版本的只读类型/连接证据；本轮未把未部署的 saved-history 改动计入生产验收，也没有向生产写入保存消息。
- `channels.getMessageAuthor` 现在先验证认证和 nil 请求，再经 `MessageGetUserMessage` 确认调用者拥有的消息属于目标频道，经 `UserGetMutableUsers` 回填真实用户作者；未命中、频道匿名作者和 provider 缺失分别返回 `userEmpty` 或明确错误。focused provider/error tests 已通过；没有把该项提升为生产级验收。
- Stories 的只读与本地状态边界已补齐：`getPinnedStories`、`getStoriesArchive`、`getStoriesByID`、`toggleAllStoriesHidden`、`readStories`、`incrementStoryViews`、`getStoryViewsList`、`getStoriesViews`、`togglePeerStoriesHidden`、`getStoryReactionsList`、`togglePinnedToTop` 现在读取/更新当前用户的持久化故事记录，并拒绝非本人 peer；空故事仍返回协议允许的空 typed envelope。故事发布、跨用户读取、媒体 provider 和跨设备更新仍未实现，因此仅计组件验收。
- Stories album 的创建、更新、排序、删除、读取和按 album 读取故事也已接入同一持久化 store；所有 story ID 和 album ID 都在写入前验证，跨用户 peer 仍失败关闭。故事媒体发布和跨设备更新仍未接入。
- 当前 CSV 台账仍为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=339`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=141`、`PARTIAL_COMPONENT_VERIFIED=117`、`PARTIAL_ISOLATED_DB_VERIFIED=34`、`PARTIAL_SESSION_VERIFIED=151`、`ISOLATED_COMPONENT_VERIFIED=10`、`ISOLATED_DB_VERIFIED=8`、传输包装 11、手动分发待验收 2。生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=350`、`NOT_ACCEPTED=141`、`PARTIAL_COMPONENT_ONLY=111`、`PARTIAL_ISOLATED_DB_ONLY=29`、`PARTIAL_ISOLATED_SESSION_E2E=153`、隔离组件 10、隔离库 8、传输包装 11。完整生产级验收仍为 **0/813**。
- `channels.setMainProfileTab`、`channels.setEmojiStickers` 仍只写共享 KV，缺少频道权威读取、同步和重启后的业务回读，因此继续保持未验收；短信/邮件 provider、真实支付结算、Passkey RP、第二 DC、原生频道完整 provider、礼物库存结算和通话媒体控制面仍是外部阻塞。

## 2026-10-02 原生频道方法级复核（本轮）

- 频道设置、论坛开关、按成员删除历史和频道照片这 5 个曾标成静态缺口的方法已经有可追溯的 APIFull 持久化路径：`channels.toggleSlowMode`、`channels.toggleParticipantsHidden`、`channels.toggleForum`、`channels.deleteParticipantHistory`、`channels.editPhoto`。本轮没有把它们提升为生产级通过；台账从 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP` 调整为隔离库部分验收。
- 隔离库命令：`APIFULL_MYSQL_DSN='root:<isolated-password>@tcp(127.0.0.1:13306)/teamgram_audit?charset=utf8mb4&parseTime=true' go test ./app/bff/apifull/internal/core -run 'Test(ChannelSettingsRoundTrip|ChannelsDeleteParticipantHistoryDeletesStoredMessages|ForumTopicsCreateGetDelete|ForumChannelAuthorization)$' -count=1` 通过。
- 新增 `TestChannelsEditPhotoPersistsUploadedPhotoAndClear`：fake DFS 返回带尺寸和视频尺寸的 `Photo`，测试确认 DFS 请求 creator/file、`apifull_channel` 的 photo 字段、Layer 229 `ChatPhoto` 字段和可逆清空；命令同样只连接 `teamgram_audit`。
- 生产目标 `backend-backend-1` 只做只读 schema 检查，确认 `apifull_channel` 已有 `slowmode_seconds`、`participants_hidden`、`photo_id`、`photo_dc_id`、`photo_has_video` 列；没有向生产频道写设置、删历史或上传照片。
- 同一隔离库回归还覆盖 `account.updateColor`、`help.getPeerColors`、`help.getPeerProfileColors`：普通颜色请求已映射到 User 服务，内置个人/资料调色板能返回并按 Layer 229 编码；collectible 颜色和动态目录仍失败关闭。
- 现有 `channels_test.go` / `forums_test.go` 也确认了 `channels.getAdminedPublicChannels` 的公开频道筛选，以及 `channels.toggleViewForumAsMessages` 的设置持久化和 `updateChannelViewForumAsMessages` 编码；这两项从仅路由调整为隔离库部分证据。
- `channels.checkUsername` 补上了认证、nil/服务可用性和未知 User provider 响应的失败关闭；focused usernames 测试覆盖可用、本人占用、他人占用和错误响应。
- 仍未解决的范围：原生频道消息/媒体 provider、真实 DFS 上传、跨进程事件投递、重启后的完整 session 回读，以及 `channels.setStickers` 所需的权威 sticker catalog。生产级完整验收仍为 **0/813**；当前台账为 `KNOWN_INCOMPLETE_STATIC=340`、`BLOCKED_BY_IMPLEMENTATION_GAP=351`。

## 2026-10-02 频道邀请开关隔离 session 复核（本轮）

- `docker/web-isolated/probe-channel-invites.ts` 以四个隔离虚拟用户完成真实 WebSocket → session → BFF 流程：`channels.toggleJoinRequest` 和 `channels.toggleJoinToSend` 分别启用、关闭并读回邀请状态；非管理员均得到 `CHAT_ADMIN_REQUIRED`，错误 access hash 得到 `CHANNEL_INVALID`。
- 探针最后删除了所有临时邀请和频道，隔离库没有留下有效业务数据。两项在台账中从仅路由调整为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`；原生频道 provider、跨进程更新、生产流量和重启持久性仍未验收。
- 当前台账仍是 813 个唯一方法；本轮后 `KNOWN_INCOMPLETE_STATIC=340`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=145`、`PARTIAL_ISOLATED_SESSION_E2E=153`、`BLOCKED_BY_IMPLEMENTATION_GAP=351`。完整生产级验收仍为 **0/813**。

## 2026-10-02 `messages.uploadImportedMedia` DFS 组件复核

- `messages.uploadImportedMedia` 已不再保存请求后返回空媒体：它校验导入会话元数据和 peer，把上传照片交给 `DfsUploadPhotoFileV2`，把上传文档交给 `DfsUploadDocumentFileV2`，并返回带真实 DFS `Photo`/`Document` 的 `MessageMedia`；DFS 错误原样传播。
- `TestMessagesUploadImportedMediaUploadsPhoto`、`TestMessagesUploadImportedMediaUploadsDocument` 和 `TestMessagesUploadImportedMediaPropagatesDFSError` 使用 fake DFS client 通过。命令为 `APIFULL_MYSQL_DSN='teamgram_web_isolated:<isolated-password>@tcp(127.0.0.1:13306)/teamgram_audit?parseTime=true' go test ./app/bff/apifull/internal/core -run 'TestMessagesUploadImportedMedia|TestImportedChats' -count=1`；测试只写隔离库，不连接 `backend-backend-1`。
- 这只是 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`：导入头解析、任务队列、消息写入服务、真实 DFS 部署和完整 session→BFF→存储回读仍未接入，不能提升为生产级验收。
- 当前台账仍为 813 个唯一方法；本轮后 `KNOWN_INCOMPLETE_STATIC=352`、`PARTIAL_COMPONENT_VERIFIED=109`，完整生产级验收仍为 `0/813`。

## 2026-10-02 `help.getUserInfo` / `help.editUserInfo` 组件复核

- 这两个方法已按调用者/目标用户作用域读写 APIFull 的 TSF 信息，保留消息实体；`TestB18User81018` 在隔离数据库环境下通过编辑后读回检查。
- 台账更新为 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`。当前证据不覆盖真实 session、用户服务权威资料源、跨进程重启或生产持久性。

## 2026-10-02 最近表情状态缓存复核

- `account.getRecentEmojiStatuses` 现在为本地最近状态列表计算稳定 hash，并在 hash 未变化时返回 `account.emojiStatusesNotModified`；`account.clearRecentEmojiStatuses` 清空列表后仍可读回空集合。
- `TestEmojiStatusRecentHashAndClear` 和 `TestEmojiStatusRoundtrip` 通过。台账更新为 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`；最近状态仍未接入用户资料权威源、跨客户端同步或生产 session 验收。
- 当前台账静态缺口为 350 项，组件部分证据为 111 项；完整生产级验收仍为 `0/813`。

## 2026-10-02 contacts.getBlocked 生产往返复核

- 使用生产现有授权会话 `136907714`，选择现有用户 `136907713` 做可逆流程：`contacts.block → contacts.getBlocked → contacts.unblock → contacts.getBlocked`。四次调用均返回正确类型（`Boolean`、`contacts.BlockedSlice`），阻断后列表包含目标用户，解除后列表恢复为空。
- 生产 MySQL 的 `user_peer_blocks` 最终只保留 `deleted=1` 的既有删除墓碑，接口查询不再返回该用户；没有留下有效阻断关系。BFF 日志记录完整请求链路，没有 error、500、超时或 panic。
- 该方法的剩余范围是频道对象 hydration、跨会话更新和重启持久性；不再属于“未实现”阻塞，台账从 `KNOWN_INCOMPLETE_STATIC` 提升为 `PARTIAL_SESSION_VERIFIED`。

## 2026-10-02 频道 typing 跨用户复核与生产 BFF 热替换（r14）

- 隔离栈使用已有认证会话（`isolated_test_07` / `isolated_test_08`）完成真实两用户流程：创建超级群、邀请成员、调用 `messages.setTyping(top_msg_id=17)`；调用返回 `Boolean`，另一用户收到 `UpdateChannelUserTyping`，`channel_id` 和 `top_msg_id=17` 均正确。首次用手机号重新登录的尝试触发 `PHONE_NUMBER_FLOOD`，没有继续生成新会话；最终验证复用了数据库中已有授权密钥。
- 生产目标 `backend-backend-1` 仅热替换 BFF，没有重建容器、清库、执行迁移，也没有重启数据库、Redis、Kafka、etcd、SeaweedFS 或其它 Teamgram 进程。运行 BFF PID 为 `623`，SHA-256 为 `495a93ef1566dc5a0e5f245f5304cde4ef7c0bb4c4f0279c76bceb8b5f9e01d1`；原二进制保存在 `/app/bin/bff.before-20261002-typing-r14`，SHA-256 为 `191ee129ea3ba0453acb34f0bd003ee2ee531888d2673ccdc13b17e45655898a`。
- 生产授权 WebSocket 只读探针在替换后通过：`help.getConfig → Config`、`users.getFullUser → users.UserFull`、`messages.getDialogs → messages.DialogsSlice`、`account.getAuthorizations → account.Authorizations`、`help.getCountriesList → help.CountriesList`，以及自身 peer 的 `messages.setTyping → Boolean`；未发送业务消息、未修改资料、未调用支付/短信/邮件或破坏性方法。探针期间无超时、500、`METHOD_NOT_IMPL`、panic 或 fatal；国家列表仍为空，保留手机号表单例外。
- `go test ./app/bff/dialogs/... ./app/bff/bff/internal/server/... -count=1` 通过。生产 BFF 启动日志显示正常注册并处理上述请求。`messages.setTyping` 现提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`；完整生产验收仍为 **0/813**。
- 尚未改变外部依赖阻塞：短信/邮件 provider、真实支付结算、Passkey RP、第二 DC、原生频道完整后端、秘密聊天跨设备密钥、礼物外部库存/结算和通话媒体控制面仍需权威依赖与独立验收。

## 2026-10-02 原生频道事件回放部署复核（最新 r12）

- 补齐原生频道的持久化事件日志：`apifull_channel_event` 与消息、`pts` 序列在同一个 MySQL 事务中记录新消息、编辑、删除和置顶事件。`updates.getChannelDifference` 现在按事件游标返回 `newMessages` 与 `OtherUpdates`，并保留旧消息快照作为回退；成员权限和每用户隐藏历史仍在查询层校验。
- `backend-backend-1` 只热替换并重启了 bff 与 biz：bff PID `1234`，SHA-256 `5e534edca76928598abc46bce27447f36c4546f10c7a8b676fcd551a634d818d`；biz PID `1115`，SHA-256 `287bd570c3f38475489a97300aa32cabd573b85dbd8fcddf660e9dbb76b9222e`。容器重启计数仍为 0；MySQL、Redis、Kafka、etcd、SeaweedFS、msg、session、sync、gnetway 和 DFS 均未重启。
- 目标生产库只读检查确认 `apifull_channel_event` 已创建，当前事件行数为 0（本轮没有写入生产业务数据）；频道消息、频道序列表仍可读。重启后通过真实 WebSocket 未授权探针再次得到 `help.getConfig` → `Config`（`this_dc=2`）。启动日志无 panic、fatal、监听冲突或 MySQL 错误。
- `go test` 的 updates 核心窄测、`go build ./app/bff/apifull/... ./app/service/biz/updates/...`、`gofmt` 和 `git diff --check` 通过。APIFull 域集成测试因环境 DSN 保护机制拒绝非隔离库，未连接生产库执行写测试。
- 台账中的 `updates.getChannelDifference` 已记录事件日志和生产部署证据，但仍是 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`：目标库尚未用真实授权会话执行一次编辑/删除后差分回放，旧消息的完整事件补录和重启业务流仍待验收。短信/邮件 provider、外部支付结算、Passkey RP、第二 DC、秘密聊天跨设备密钥、礼物外部库存和通话媒体控制面继续阻塞。

## 2026-10-02 生产目标部署复核（最新 r11）

- `backend-backend-1` 当前运行 `teamgram-server-latest:20261002-r11`，状态为 running，重启计数为 0。容器使用现有生产 MySQL `mysql:3306/teamgram`；没有清库、没有执行 `migrate-*.sql`，也没有重启 MySQL、Redis、etcd、SeaweedFS 或 Kafka。
- 从生产 `auth_users`/`auth_keys` 只读取得现有会话授权后，真实 Layer 229 探针通过 `help.getConfig` → `Config`、`help.getNearestDc` → `NearestDc`、`account.getAuthorizations` → `account.Authorizations`、`users.getFullUser` → `users.UserFull`、`messages.getDialogs` → `messages.DialogsSlice`、`help.getCountriesList` → `help.CountriesList`。
- 真实礼物 `payments.getUniqueStarGift(slug=prod-gift)` 返回 `payments.UniqueStarGift` 并完成 Layer 229 编码；修复了生产礼物缺少媒体字段时的空指针崩溃。频道只读链路也返回 `messages.Messages`、`messages.ChatFull`、`channels.ChannelParticipants`、`messages.MessageViews` 和 `updates.ChannelDifference`。
- 生产 Stars 商品目录当前为空，`payments.getStarsTopupOptions` 返回 `400 METHOD_NOT_IMPL`，这是无商品配置时的 fail-closed 行为，不计为服务端错误。生产库只读统计仍为 `users=3`、`chats=2`、`messages=62`、`apifull_channel=13`、`apifull_gift=35`、`apifull_stars_offer=0`。
- 最新工作树通过 `gofmt` 检查、`git diff --check`、`go build ./app/bff/apifull/...` 和 `go build ./app/...`。探针期间没有超时、500、panic 或 fatal；初始 `badServerSalt` 是新会话建立时的协议校正，随后请求均正常返回。
- 这轮证据仍是生产部署和具名只读业务验收，不把 813 项全量生产级验收标为完成。短信/邮件 provider、真实支付商品与结算、Passkey RP、第二 DC、原生频道事件日志、秘密聊天跨设备密钥、外部礼物结算和通话媒体控制面仍需权威依赖与独立验收。

## 2026-10-01 生产目标部署复核（最新 r4）

- `backend-backend-1` 已切换到当前工作树构建的 `teamgram-server-latest:20261001-r4`，镜像 ID 为 `sha256:c61441c9f6f9ceb4e34827bf07c13e39f0eaa7edf4654db262589d4b4759e928`。容器重建后保持运行，11 个 Teamgram 子进程均已启动，重启计数为 0。
- 容器使用生产 MySQL DSN `teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true`。没有清库，没有执行 `migrate-*.sql`，没有转换旧数据；最新代码启动时按当前 DDL 幂等补齐 `apifull_channel_admin_log`，生产库现在有 88 张表。
- 通过真实生产网关 WebSocket 只读探针：`help.getConfig` → `Config`、`help.getNearestDc` → `NearestDc`、`help.getCountriesList` → `help.CountriesList`、`help.getAppConfig` → `help.AppConfig`。没有发送消息、修改资料、支付、短信、邮件或破坏性请求。
- 部署后检查到的生产日志没有新的 panic、fatal、业务 500 或超时；之前将 WebSocket 请求误发到 TCP 端口 `10443` 的两条 `there is http transport protocol` 记录来自探针端口错误，不是服务启动错误。正确的 WebSocket 端口为 `11443`。
- 本轮新增并部署：频道成员管理员审计事件的事务内快照和 `channels.getAdminLog` 读取；`upload.getWebFile` 的真实 HTTP(S) 分块读取与私网/DNS/范围校验；`messages.uploadMedia` 的媒体/DFS 错误传播；保存消息已读游标的缺失 provider 和空响应 fail-closed。上述代码已通过 `go build ./app/...`、完整 `./build.sh` 或对应窄测后构建进镜像。
- Layer 229 台账当前为 813 行：371 项静态明确缺口、151 项仅路由未验收、136 项部分 session、97 项部分组件、27 项部分隔离库、10 项隔离组件、8 项隔离库、2 项 session 手动分发待验收和 11 项传输包装；生产验收状态为 381 项被实现缺口阻塞、151 项未验收、139 项部分隔离会话、91 项部分组件、22 项部分隔离库、10 项隔离组件、8 项隔离库和 11 项传输包装。完整生产级业务验收仍为 **0/813**。
- 短信/邮件 provider、支付 sandbox 与权益结算、Passkey RP、真实第二 DC、原生频道事件 provider、秘密聊天跨设备密钥、礼物库存结算和通话媒体控制面仍是外部依赖阻塞；当前配置没有凭据，因此保持 fail-closed，不把固定空结果计为实现。

## 2026-10-01 生产目标部署复核（r3 历史记录）

- `backend-backend-1` 已替换为当前工作树构建的镜像 `teamgram-server-latest:20261001-r3`（image ID `sha256:5766f3c911758268e9b5feb27937b49c2bb88d6cd97fab2655e67c1f50ddc0f6`），容器保持运行，11 个 Teamgram 子进程均已启动。
- 使用现有 `teamgram` MySQL 连接完成只读协议探针：`help.getConfig` → `Config`、`help.getNearestDc` → `NearestDc`、`help.getCountriesList` → `help.CountriesList`（国家列表为空，保留手机号表单例外）、`help.getAppConfig` → `help.AppConfig`。探针没有调用写入或破坏性方法。
- 使用生产库中已有的授权密钥完成一次真实授权只读探针：`users.getFullUser` → `users.UserFull`、`messages.getDialogs` → `messages.DialogsSlice`（返回 2 个对话）、`account.getAuthorizations` → `account.Authorizations`（返回 1 个会话）。未发送消息、未修改资料、未触发支付/短信/邮件或破坏性方法。
- 最新 APIFull 初始化在启动时补齐了生产库原先缺少的 `apifull_report` 与 `apifull_channel_message_content_read` 两张表；没有执行 `migrate-*.sql`、清库或旧数据转换。生产库当前 87 张表，所有仓库 DDL 中声明的表均可见。
- 生产级完整链路仍为 **0/813**：以上是部署/协议/类型和只读配置证据，不等于业务数据写入、重启持久性、权限边界或外部 provider 验收。短信/邮件 provider、支付 sandbox、Passkey RP、跨 DC、原生频道 provider、秘密聊天跨设备密钥、礼物结算和通话媒体控制面仍按台账阻塞记录。
- 为消除后端默认 `jaeger:4317` 的 DNS 错误，已在 `backend_teamgram_net` 启动同网的 Jaeger OTLP collector；它不连接业务数据库，也没有改变业务数据。

## 当前范围和状态

- 唯一验收清单是 `../telegram-api/methods.json`，共 813 个唯一方法。生成的同步服务 proto 有 869 个 RPC 声明；本审计不把这 56 个清单外声明混入 813 项。
- 当前静态关联结果：793 个方法有已注册的 BFF RPC 路由；9 个新 Layer 229 方法由 session 的 `layer229.Dispatch` 手动处理；11 个 `initConnection` / `invoke*` 是 session 解开的传输包装。
- 9 个手动分发方法中，6 项已做隔离组件测试、欢迎消息读取部分通过；`ephemeral.sendMessage` 和 `ephemeral.deleteMessage` 仍未验收。
- 本轮生产验收状态以台账新增的 `production_acceptance_status` / `production_acceptance_evidence` 为准。完整的 BFF/session → 业务服务 → 持久化或外部 provider → 读回/权限验证链路目前为 **0/813**；旧的 641 个类型化结果、126 个参数拒绝是构造/参数扫描，不等于业务验收。
- 当前台账审计状态列有 422 项明确静态缺口、178 项仅确认路由、10 项隔离库验收、18 项部分隔离库验收、10 项隔离组件验收、21 项部分组件验收、141 项部分隔离 session 验收、2 项 session 手动分发待验收和 11 个传输包装。生产验收列另有 418 项被实现缺口阻塞、179 项未验收、10 项隔离库证据、19 项部分隔离库证据、10 项隔离组件证据、22 项部分组件证据、144 项部分隔离 session 证据及 11 项传输包装。TL codec 另有 36 个 Layer 229 构造器缺口。上述隔离测试和组件测试都没有被计作生产级端到端通过。

## 2026-10-01 当前工作树隔离预发布验收（不计生产通过）

- 从工作树 `708fb9536f60e15d9bf4f1c302aa4251bfefab19` 构建了新的隔离 Teamgram 镜像（本地 image ID `sha256:39a9f29c2e39a9b24f60688b52cc9236c8a19398e133a0b755cfdbd8e02e916`），启动独立 MySQL、Redis、Kafka、etcd、MinIO、session、BFF、msg 和 gateway。该栈使用临时卷，未连接 `backend-backend-1` 或默认 `3306/teamgram`。
- 14 个隔离 fixture 账号通过真实 WebSocket → session → BFF 路径完成登录，联系人、私聊、群、频道、搜索、已读、频道差分、统计和群通话控制流程通过，结果为 `business-flow acceptance passed: current-worktree-e2e-1790811674`。
- 发送标记为 `restart-e2e-1790811794` 的真实私聊消息后重启 Teamgram 主容器；隔离 MySQL 中对应两条消息记录重启前后均存在，重新登录后通过 `messages.getHistory` 读回同一消息（ID 19）。
- 重启后的隔离服务保持运行，MySQL 74 张表可读；本轮运行日志未出现 panic、fatal、`METHOD_NOT_IMPL` 或未预期的 error。测试完成后已删除该隔离栈及其卷；本地构建镜像仍保留作候选 artifact。
- 这组证据只证明当前工作树的隔离预发布链路和重启持久性，不能提升台账的生产验收计数。短信/邮件 provider、支付 sandbox、Passkey RP、跨 DC、原生频道、外部礼物/通话 provider、负载/故障恢复以及真实生产数据库仍待具名目标和授权后验收。

## 2026-09-29 当前隔离复核

- 群成员状态修复已热替换到唯一隔离 BFF，运行 SHA-256 为 `05fe5dda34b696cc45d609930b09def74120776f0b307d9d2fb54a7028f7c2f9`，仅重启隔离 BFF；回滚副本为 `/app/bin/bff.pre-group-fullchat-20260929`。msg、session、gnetway、数据库、Kafka 和生产服务均未重启。
- 本轮真实虚拟用户探针覆盖账户授权/网页授权/密码设置、通知设备和联系人开关、基础群成员删除权限、私聊/群消息读取搜索和未读提及、频道消息生命周期。所有证据均按 `PARTIAL_*` 记录；完整生产级验收仍为 `0/813`。
- 当前隔离运行态已封存为本地镜像 `teamgram-web-isolated-runtime:20260929-bff-05fe5dda`，包含 BFF SHA `05fe5dda...` 和 msg SHA `7f789879...`；外部 MySQL、Redis、etcd、Kafka、MinIO 卷不在镜像内。
- `messages.acceptUrlAuth` 的 Layer 229 URL 字段源码修复和回归测试已加入工作区，但未部署到隔离 BFF；其余新消息读取源码修复同样只做组件/代码验证，不能据此提升生产验收等级。

## 2026-09-28 当前候选复核

- 当前隔离 BFF 候选 SHA-256 为 `bc939e06588487de35cb8f223b4ac5ead2d7f71ce0c828d84244d0fe2e77465c`，容器内只替换并重启了 BFF，回滚副本为 `/app/bin/bff.pre-passport-final-20260928-r2`；数据库、Kafka、msg、session 和其它服务没有重启。
- `verify-business-flows.sh` 以 14 个隔离 fixture 用户重新通过（`isolated-e2e-1790600181702-15891`），覆盖联系人、私聊、群、频道、搜索、已读、草稿、置顶、投票、频道差分和统计。随后通过论坛授权、频道消息生命周期、投票、群邀请、联系人位置隐私、资料/对话、反应、群通话、媒体上传下载、频道成员/链接、定时消息、对话过滤器和跨 DC 上传探针。并行启动导致群邀请探针首次在建群时连接重试失败，单独重跑完整通过；这不是服务端错误。
- 当前候选真实复核的结果已写入 `LAYER229_METHOD_LEDGER.csv`。被提升的条目只使用 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`，明确保留原生频道 provider、重启持久性、外部支付/短信/邮件/AI 和完整 Layer 229 参数语义限制。未实现或无权威后端的方法继续保持 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP` 或仅路由状态。
- 生产级验收标准：用有效业务数据调用真实 session/BFF 路径，检查 Layer 229 返回类型和字段；对写操作读取持久化结果并检查重启后的持久性；覆盖成员/角色/租户边界及无效请求；检查事件投递、错误传播、超时和幂等。外部支付、短信、邮件、AI 等依赖必须使用相应 sandbox/provider；本地模拟或空参数 400 不算通过。
- 本轮没有向 `backend-backend-1` 或其默认 `teamgram` 数据库发送 RPC、重启服务或执行破坏性方法。额外启动的 `teamgram-web-isolated-*` 项目拥有独立卷、十四个 fixture 用户和内部服务网络；对它的固定测试码登录会写入该项目私有 `teamgram` 卷，不能与此前的专用审计库或生产数据混同。

## 本轮十四用户真实业务流验收

- 隔离 fixture 新增 `isolated_test_07` 到 `isolated_test_14`（固定测试码 `12345`），共 14 个可独立登录的虚拟用户。fixture SQL 可重复执行，新增用户只写入隔离 MySQL。
- `verify-business-flows.ts` 以十四个独立 GramJS session 逐个登录，使用真实 WebSocket → session → BFF 路径完成联系人添加/读取、屏蔽/解除屏蔽及列表读回、私聊发送/历史/搜索/全局搜索/已读/编辑/置顶/转发、草稿保存/读回/清除、对话未读标记和对话列表。
- 同一批用户创建并清理十四人基础群，邀请另外十二名成员，验证群发消息、群内搜索、已读回执和完整群成员列表；创建并清理频道，验证邀请、发送、历史、指定消息读取、频道阅读数、搜索、差分、详情、成员列表和非管理员编辑权限拒绝。
- 本次隔离端到端结果：`business-flow acceptance passed: isolated-fourteen-final-1790462568`。脚本 `node --check`、`git diff --check` 通过；运行期间日志只出现预期的 `CHAT_ADMIN_REQUIRED`，未出现 panic、`METHOD_NOT_IMPL`、`PeerStories.Encode` 崩溃或超时。
- 这组业务流覆盖的是当前可用的真实用户/群/频道路径，不等于 813 个方法全部实现。台账中的 435 个明确静态缺口、支付/短信/邮件/AI/原生频道事件日志等仍按各方法记录处理；空参数 400、破坏性方法跳过和外部 provider 缺失不计为成功。

## 本轮隔离运行验收

- 隔离 gateway 只发布 `127.0.0.1:31443`。Docker Desktop 不会从纯 `internal` 网络创建实际 port endpoint，因此 gateway 保留在 `isolated` 内部网络，同时独占项目作用域的 `frontdoor` 网络；该网络只含 gateway，不连接生产网络。重建 gateway 后 `docker port` 显示回环绑定，`/apiws` WebSocket upgrade 返回 `101`。
- WebSocket 接收路径现在先将同一回调内的所有 binary payload 写入持久缓冲区，再创建并循环解码 MTProto codec。回归测试覆盖 64-byte obfuscated header 与首帧拆分，以及首帧自身分段后 `ErrUnexpectedEOF` 的续包解码；这避免 GramJS 分两次 `WebSocket.send()` 时丢失首包。
- 新鲜 Chromium context 通过真实 WebSocket/session/BFF/authsession 路径，用隔离 fixture 的固定测试码完成 `auth.sendCode` 和 `auth.signIn`，进入主界面。该证据只覆盖测试用户分支；SMS 投递、防刷、MFA、可信 API 凭据注册、跨 DC 与生产持久性仍未验收。前端初始化仍会调用已知缺失的贴纸、Stars 和 passkey API，并收到明确 400；这些 fail-closed 响应不影响该登录验收。
- `account.saveMusic`、`account.getSavedMusicIds` 和 `users.getSavedMusic` 新增 focused component 测试。它们验证输入/认证、后端错误和 nil 响应、hash/not-modified、分页、隐私及返回顺序；没有把 stub 测试计为持久化或生产验收。

## 2026-10-07：Saved Music 真实生产回环与字段修复

- 生产只读检查发现 `user_saved_music` 的一条旧测试行是 `saved_music_id=0`，不能把它当成有效歌曲。新增 `docker/production/probe-saved-music-roundtrip.ts`，使用生产用户 `136907713` 和已存在的 `documents` 行执行可逆流程。
- 随后确认该 `saved_music_id=0` 行不可能由当前合法 `account.saveMusic` 请求产生，且会污染列表语义；已在生产 MySQL 中仅按 `user_id=136907714 AND saved_music_id=0 AND deleted=0` 将这一行标记为 `deleted=1`、`order2=0`。没有改动用户资料或其它收藏。复跑只读探针 `probe-saved-music-ids-readonly.ts` 返回 `account.SavedMusicIds` 且 `ids=[]`、`writes=0`。
- 修复 `app/service/biz/user/internal/core/user.saveMusic_handler.go`：取消收藏时按 `v.SavedMusicId == in.GetId()` 匹配目标文档，原实现错误地拿文档 ID和用户 ID 比较，导致有效收藏无法取消。新增 `TestSavedMusicEntryMatchesRequestedDocument` 覆盖目标文档、用户 ID 误匹配、零 ID 和 nil 行。
- `backend-backend-1` 已更新为 `teamgram-server-latest:20261007-r17-prod`，仅替换 `biz`（SHA-256 `80ffc4734f664eb7d2f6efecc3f8792b6ff62dd88403268c3ca085eec67c8248`）；容器 `running`、`restart=0`、`oom=false`，数据库、Redis、Kafka、etcd 和其它进程未重启。
- 真实 DC2 WebSocket → gateway → session → BFF → User service/MySQL 流程返回：保存 `Boolean`，读取 `account.SavedMusicIds` 且包含文档 ID `2107387320960290816`，按同一 hash 返回 `account.SavedMusicIdsNotModified`，取消 `Boolean`，再读为空。生产 MySQL 仅留下该用户该文档的 `deleted=1` 软删除行，active 数据为 0。
- 因此 `account.saveMusic`、`account.getSavedMusicIds`、`users.getSavedMusic` 和 `users.getSavedMusicByID` 从 `PARTIAL_COMPONENT_ONLY` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。生产探针只使用用户自己的 `InputUserSelf`，跨用户隐私、错误 access hash、完整分页边界和重启持久性仍需独立验收；用户 `136907714` 的无效 `saved_music_id=0` 行已软删除。

## 本轮已修复并验收

| 方法 | 改动和证据 | 状态 |
| --- | --- | --- |
| `channels.editTitle` | 现在直接更新 MySQL 中频道标题，要求调用者是频道创建者；读取回写结果、验证非创建者被拒绝，并在 Layer 229 编码返回的 Updates。 | `TestChannelsEditTitleRoundTrip` 在隔离库 `teamgram_audit` 通过。 |
| `messages.getPersonalChannelHistory` | 先通过 UserClient 读取目标用户的 `personal_channel_id`，再从 apifull 频道消息表读取，并应用 `min_id`、`max_id` 和 `limit`。无个人频道时才返回空列表。 | `TestMessagesGetPersonalChannelHistoryRoundTrip` 在隔离库通过，且返回消息与分页结果都通过 Layer 229 编码；UserClient 用测试替身。若个人频道只存在于原生消息服务而非 apifull 表中，仍会返回 `CHANNEL_INVALID`，因此标为部分验收。 |
| `auth.checkPassword` | 校验传入的 SRP `M1`，密码状态和挑战读取同一 `apifull_kv`，挑战限时且只能消费一次；登录、未来授权令牌和 QR 登录在授权前都查该密码状态。密码恢复现在保存客户端 verifier，不会覆盖 KDF 状态。 | SRP 正确证明、错误证明、过期及重放测试通过；MySQL 一次性消费测试只使用 `teamgram_audit`。还没有真实授权会话的完整登录测试，因此只标组件部分验收。 |

## 本轮补充的回读验收

- 在 `teamgram_audit` 运行了 8 个既有用例：poll 投票/结果/投票人、待办追加与完成状态、自动保存设置、频道设置和频道已读游标、个人频道历史。均通过；其中频道本地数据路径仍按局部验收处理。
- `account.getAutoDownloadSettings` / `account.saveAutoDownloadSettings` 新增隔离存储组件测试，验证低流量配置回读、64 位大小字段和用户隔离；通过。此测试用内存隔离存储，不代表 MySQL 往返验收。

## 本轮静态修复，待真实数据验收

- 群邀请创建、读取加入者、编辑和删除现在检查群成员状态及 `CanInviteUsers`；编辑或清理其他管理员的链接还要求 `CanAdminAddAdmins`。邀请链接必须属于请求中的群。
- `messages.editExportedChatInvite`、`messages.deleteExportedChatInvite` 和 `messages.deleteRevokedExportedChatInvites` 现在传播数据库错误，不再在写入失败时报告成功。
- `messages.getExportedChatInvites` 的外部 BFF 路径会验证调用者权限及传入 `admin_id`。底层 chat RPC 请求没有调用者 ID 字段，仍需要补上服务级身份校验。
- `messages.getChatInviteImporters` 仍未实现 `q` 搜索；授权管理已补齐，但必须用隔离群组数据验收授权管理员、普通成员、非成员和跨群链接。
- 这批邀请修改仅通过包级构建验证，没有真实 RPC/数据库往返证据，台账未标为功能验收通过。本轮没有向当前运行中的服务发送请求。
- `auth.signIn`、`auth.sendCode` 的未来授权令牌分支以及 QR 导入/导出现在读取同一密码状态；原来 `Plugin == nil` 时的 2FA 绕过已堵住。授权完整流程仍没有端到端验证。

### Layer 229 返回编码

- 会话层按客户端层编码 RPC 返回值。`GetClazzID` 原先只查请求层和 layer 0；Layer 229 没有专用映射的旧构造器会得到 ID 0，`messages.messages` 和分页结果因此无法编码。现在先查精确层，再仅对 Layer 229 回退到 Layer 228；其他层的查找规则不变。
- `proto/mtproto/layer229_encode_test.go` 覆盖 `messages.messages`、`messages.messagesSlice`、`messages.messagesNotModified`，以及消息、用户、频道的嵌套编码。隔离库的个人频道历史和标题修改测试也实际编码了 RPC 返回对象。
- `TLRpcResult.Encode` 以前忽略内层结果的编码错误，session 又忽略外层错误，可能把半截响应当成功入队。现在两层都传递错误；session 记录请求 ID 和错误后发送 `INTERNAL_SERVER_ERROR`。`rpc_result_test.go` 与 session 的 `session_rpc_result_test.go` 覆盖错误传播。
- `ephemeral.getWelcomeMessages` 的嵌套 `EphemeralMessage` 以前使用 Layer 228 构造器。Layer 229 现在按其字段顺序编码为 `0xdd27bee9`，覆盖本地数据模型能表示的字段；协议回归检查了这个嵌套构造器。当前模型仍无法表达全部 Layer 229 ephemeral 字段。
- 更广泛的 schema 对比发现 37 个 Layer 229 构造器 ID 在当前 proto 源码中缺失；欢迎消息编码补上其中 1 个路径，仍有 36 个构造器没有 Layer 229 编码/解码支持：
  - 消息动作和更新：`messageActionStarGiftUnique`、`messageActionChatJoinedViaCommunity`、`updateEphemeralBotCallbackQuery`。
  - 发送动作：`sendMessageTextDraftAction`、`inputSendMessageRichMessageDraftAction`、`sendMessageRichMessageDraftAction`、`sendMessageStopDraftAction`。
  - 按钮和 markup：`keyboardButton`、`replyInlineMarkup`、`buttonTypeDefault`、`buttonTypeRequestPhone`、`buttonTypeRequestGeoLocation`、`buttonTypeRequestPoll`、`buttonTypeRequestPeer`、`inputButtonTypeRequestPeer`、`buttonTypeSimpleWebView`、`inlineButtonTypeUrl`、`inlineButtonTypeUrlAuth`、`inputInlineButtonTypeUrlAuth`、`inlineButtonTypeWebView`、`inlineButtonTypeCallback`、`inlineButtonTypeGame`、`inlineButtonTypeBuy`、`inlineButtonTypeSwitchInline`、`inlineButtonTypeUserProfile`、`inputInlineButtonTypeUserProfile`、`inlineButtonTypeCopy`、`inlineButtonTypeDisabled`、`keyboardInlineButton`、`keyboardInlineButtonRow`。
  - 富文本和网页块：`textButton`、`pageBlockBlockquote`、`pageBlockButtonRow`、`pageBlockDocument`、`pageButton`。
  - 礼物发票：`inputInvoiceStarGiftResale`。
- 这些构造器缺口还可能影响静态 handler / 协议语义缺口以外的方法；第三次复核时台账有 451 项静态缺口和 323 项仅确认路由，当前数量见下方第四次复核。

回归还通过了 Layer 229 构造器编码测试、SRP challenge 测试、恢复密码 verifier 保留测试，以及隔离库中的频道标题、个人频道历史、SRP challenge 原子消费测试。`go build ./app/bff/...` 通过，也修复了 `acctKey` 缺失和两处频道数据读取漏传用户 ID 的编译问题。测试只写入 `teamgram_audit`；未运行该文件中的频道删除测试。

## 本轮新增的静态缺口

- **认证：** `auth.exportAuthorization` / `auth.importAuthorization` 已在隔离 WebSocket/session/BFF/Redis/authsession 路径完成授权转移和后续上传验证；`dc_id` 仍只有语法校验，且 Redis 消费与目标 key 绑定不是同一事务。`auth.exportLoginToken` 仍没有可信 API 凭据注册表或多 DC 迁移验收。
- **账号与联系人：** `account.updateStatus` 更新 last-seen 后没有把在线/离线状态推送给联系人；`account.getNotifySettings` 现在验证用户和基础群成员；频道检查需要默认配置缺失的 NotificationPlugin。`contacts.addContact` 不支持 `InputUserFromMessage`；`contacts.deleteContacts` 已传播用户读取和删除写入错误，并拒绝空、自身及不支持的输入；`InputUserFromMessage` 仍未实现。
- **频道与消息：** `channels.getMessages`、`channels.readHistory` 只读写 APIFull 本地频道表，不校验成员或 access hash，也没有原生频道回退；`messages.getFullChat` 的置顶消息、邀请、BotInfo 和通话/加入请求字段仍为 nil 或固定值；`messages.readMessageContents` 没有确认一批消息都属于同一 peer。
- **同步与媒体：** `messages.toggleDialogPin`、`messages.reorderPinnedDialogs`、`messages.toggleSavedDialogPin` 在频道 peer 分支没有补齐同步所需的频道对象；`messages.uploadMedia` 将外链照片/文档、游戏和发票转换成 `MessageMediaUnsupported`。

这些是代码路径中确认的部分实现或缺失分支，不代表 459 项都完全空实现；具体原因和路径在台账中逐项记录。

## 已确认的功能缺口

### 频道方法

`app/bff/apifull/internal/core/channels.go` 中下列 Layer 229 方法仍只保存请求或返回固定结果，没有实现对应状态变化：

- `channels.editAdmin`、`channels.editPhoto`、`channels.joinChannel`、`channels.leaveChannel`、`channels.inviteToChannel`、`channels.deleteChannel`
- `channels.exportMessageLink`、`channels.editBanned`、`channels.getAdminLog`、`channels.setStickers`、`channels.readMessageContents`
- `channels.getGroupsForDiscussion`、`channels.setDiscussionGroup`、`channels.editLocation`、`channels.getInactiveChannels`、`channels.deleteParticipantHistory`

频道模型目前只保存创建者和基础设置。尚无成员、管理员、封禁、邀请、审计日志、讨论组、位置或消息内容已读记录。

### 消息与频道数据路径

- `app/bff/messages/internal/core/messages.getHistory_handler.go` 对所有 `PEER_CHANNEL` 无条件调用 `channelview.History`。它不检查访问哈希或成员身份，也没有回退到原生消息表；有效的原生频道因此不能读取其真实历史。apifull 侧的频道历史、搜索和阅读接口也只认识 `apifull_channel_message`，其中 `channels.getMessages` 和 `channels.readHistory` 还没有成员/access-hash 校验。
- `messages.getPersonalChannelHistory` 目前只覆盖 `personal_channel_id` 指向 apifull 自有频道表的用户。原生频道的 personal channel 历史仍缺少可用读取路径；`apifull` DAO 也没有消息服务客户端。

### 电话会议、礼物、付费和统计

- 群通话/会议已有局部本地状态；录制现在至少持久化 creator-authorized 控制状态，但没有录制器或媒体消费者。邀请、丢弃、编辑参与者/标题、会议广播、定时启动、屏幕共享和通话消息仍有跨客户端/媒体控制缺口。代码见 `group_calls.go`、`conference_calls.go`。
- 礼物目录、拍卖和部分读取返回固定空集合；通用写入把参数中的文本存作本地礼物记录，没有礼物库存、所有权转移、升级或扣款流程。代码见 `gifts.go`。
- 付费消息收入固定为 0；价格和免付例外没有接到消息发送与结算；扩展媒体方法只把请求 ID 回显到 Updates。代码见 `paid_message.go`、`paid_media.go`。
- `payments.getStarsRevenueStats`、`payments.getStarsRevenueWithdrawalUrl` 和 `payments.getStarsRevenueAdsAccountUrl` 对合法请求返回 `METHOD_NOT_IMPL`，不再伪造零值或空 URL。频道与超级群统计仍有多项硬编码零，公开转发仍为空。代码见 `stars.go`、`statistics.go`。

### 其他静态确认缺口

- `messages.composeMessageWithAI` 返回空文本，没有生成服务。
- `channels.reportAntiSpamFalsePositive` 现在校验频道成员、消息 ID 和 canonical 频道消息，并写入幂等 `apifull_report` moderation intake；审核消费者/处置仍未接入。`channels.toggleAntiSpam` 已改为写入 canonical 频道反垃圾设置，但消息拦截消费者和生产验收仍未完成。
- `contacts.blockFromReplies` 现在从权威消息服务解析当前用户收到的私聊消息作者，通过 UserBlockPeer 持久化屏蔽，按 flags 调用 msg 删除消息/历史并把 spam 写入持久化举报 intake，最后通过 SyncClient 推送 `updatePeerBlocked`；缺少任一 provider 时 fail-closed。尚未做隔离 session、真实数据库和生产往返验收，消息服务对入站私聊归属及删除/举报副作用仍需端到端核对。`messages.readDiscussion` 现在按用户、peer 和根消息保存单调递增的 `read_max_id` 并返回 `BoolTrue`；讨论更新推送和完整消息语义仍未接入。
- `messages.reportMessagesDelivery` 只保存第一个消息 ID，没有执行实际业务副作用。`messages.startBot`、`help.getDeepLinkInfo` 和 `help.getRecentMeUrls` 没有 bot 启动、深链解析或最近链接存储后端；现在明确返回 `METHOD_NOT_IMPL`，不再返回伪造成功结果。
- `account.uploadRingtone` 已校验文件名并通过 DFS 返回真实 `Document`，但保存铃声目录仍是本地状态；`contacts.exportContactToken` 没有生成 token URL，`contacts.importContactToken` 只保存输入 token 并返回 `UserEmpty`。
- 邮箱验证与找回只在部署配置 `EmailProvider: http`、`EmailSendCodeUrl` 和有效 provider 凭据后使用真实 HTTP 投递；未配置时明确返回发送不可用，不生成可用的成功结果。HTTP provider 校验 endpoint、超时和响应状态，`ProviderRetryCount` 只重试网络错误和 5xx，4xx/3xx/空 endpoint fail-closed。APIFull 的 `account.resendPasswordEmail` / `account.confirmPasswordEmail` 现在复用 Redis challenge 和同一 HTTP email provider：重发保存一次性 challenge ID，确认原子消费验证码并清除 pending 状态。当前仍没有真实邮件 endpoint/Redis 凭据、完整密码邮箱流程或生产验收。
- `account.resetPassword` 现在把原始请求时间、requested-wait 截止时间和 failed-wait 重试时间写入共享密码状态，并在顺序重复调用时复用已保存的时间；首次请求并发时仍有非原子写入竞态。仅有内存存储组件测试，重置完成路径和生产持久化尚未验收。

### 本轮继续静态复核新增的问题

- **认证与隐私：** SRP 证明校验和 session 2FA 门禁已补上；四个核心 SRP 校验场景和 MySQL 挑战原子消费已在组件/隔离库验证，但没有完整手机号登录或 QR 登录实测。仍缺 `auth.sendCode` / `auth.resendCode` 的手机号防刷和数据中心迁移，`auth.signUp` 的手机号防刷和注册名策略校验；邮箱验证码/密码恢复没有发送通道，邮箱确认还接受任意非空 code；`account.resetPassword` 的顺序重复调用已复用持久化等待时间，但首次请求的并发竞态、完成重置流程和生产持久化尚未验收。`messages.getOutboxReadDate` 现在校验双方 `STATUS_TIMESTAMP` 隐私、消息归属和目标 peer；七天过期阈值采用客户端/TDLib 的 604800 秒默认值，服务端动态配置未接入，且高级隐私规则仍返回 `METHOD_NOT_IMPL`。群邀请权限已做静态修复，但底层 `messages.getExportedChatInvites` RPC 仍缺少调用者 ID，`messages.getChatInviteImporters` 的 `q` 搜索未实现，相关权限边界尚未用真实数据验收。`account.deleteAccount` 现在会在 user service 事务中清理账户拥有的资料、联系人、隐私、设置和已保存数据，并在 BFF 层传播会话撤销/通知错误；消息、对话、媒体等其他服务的数据仍需各自权威清理接口。
- **消息和更新：** `messages.searchGlobal` 只接受后端支持的未过滤首屏请求；folder/filter/date/cursor 等暂不支持字段返回 `METHOD_NOT_IMPL`，频道对象 hydration 缺失也会失败关闭。`messages.getMessages` 现在校验调用者和消息 ID，传播消息、用户、群组服务错误；消息涉及频道时因没有频道对象解析器返回 `METHOD_NOT_IMPL`。`messages.getCommonChats` 现在校验目标用户，合并基础群和 APIFull 频道的共同成员关系，回填两类聊天并支持分页；APIFull 频道路径已通过真实隔离 session 验收，原生频道仍没有权威 provider。`messages.getSavedHistory` 通过 `channelview.ChatsByID` 只回填 APIFull 本地频道；原生频道对象仍不会出现在响应中。`messages.getUnreadMentions` 的频道请求明确返回 `CHANNEL_UNREAD_MENTIONS_UNSUPPORTED`：消息 DAO 只实现基础群未读提及查询，APIFull 频道存储没有提及实体或按用户记录的未读提及状态。`messages.getMessagesViews` 在没有权威阅读计数时返回 `METHOD_NOT_IMPL`，不再合成零值成功。`messages.getMessageEditData` 的频道分支可读回 APIFull 存储的文本消息，并验证 access hash、成员、创建者编辑权限和消息存在性；原生频道消息与媒体 caption 仍不支持。`updates.getDifference` 的频道对象收集分支为空。Layer 229 的 `messages.getMessageReadParticipants#2c6f97b7` 现在校验消息归属和请求者频道 dialog，并通过 native `dialog.getChannelMessageReadParticipants` 返回原生频道阅读游标命中的用户 ID；APIFull 频道存储保持独立，不会被误当作该 provider。基础群路径同样校验成员、消息归属、nil 响应和 provider 错误。`messages.getMessageReadParticipants#31c1c44f` 复用频道阅读游标 provider，但 `ReadParticipantDate.date` 仍无权威逐用户时间：无参与者时返回空向量，有参与者时 fail-closed 为 `METHOD_NOT_IMPL`；基础群 read history 只持久化 `read_inbox_max_id`，`message_read_outbox.read_outbox_max_date` 仅写入 `PEER_USER`，不能代替群已读时间。定向 helper 测试覆盖消息归属、空向量和无时间戳时 fail-closed；native 频道 provider 尚无完整生产验收。`messages.hidePeerSettingsBar` 现在传播 `user.deletePeerSettings` 错误，并对 nil 回复返回 `INTERNAL_SERVER_ERROR`；用户核心也会返回 DAO 错误。DAO 对 `user_peer_settings.hide` 执行 SQL 更新，并且仅在 SQL 成功后失效缓存。当前 `updatePeerSettings` 负载只有 `PeerSettings` 字段，没有隐藏提示条状态，不能准确同步该状态；`messages.readDiscussion` 已保存讨论已读游标，但仍没有对应更新推送。
- **列表与资料：** `contacts.getBlocked` 现在实际使用 50 条上限、offset 分页和带总数的 `contacts.blockedSlice`，并回填可用群对象；频道对象仍依赖插件，完整服务链路未验收。`users.getSavedMusic` 现在按完整列表计算 hash 并支持 offset/limit 分页；尚无真实用户数据验收。`users.getFullUser` 仍有共同频道计数及基于共同群组的隐私判定缺口。`account.setAccountTTL` 现在传播用户服务的写入错误；`messages.getPeerSettings` 现在传播读取错误并回填用户和基础群，频道对象仍依赖默认未注入的插件。
- **联系人状态写入：** `contacts.block` 和 `contacts.unblock` 现在传播用户服务写入错误；尚无真实屏蔽/取消屏蔽往返验收。
- **频道对象的条件缺口：** `contacts.resolveUsername` 和 `account.getNotifyExceptions` 仍依赖频道 `Plugin`；生产配置缺少插件时，这些频道请求会失败关闭。`contacts.search` 已支持当前用户所在基础群的搜索，但频道结果仍需 `ContactsPlugin`，生产 contacts 服务未配置时会失败关闭。`account.getPrivacy` / `account.setPrivacy` 对规则涉及的频道没有权威频道 hydration 路径。

以上静态缺口不是线上复测结论；带有插件条件的缺口是否命中还取决于部署配置。SRP 登录本轮只完成了组件和存储验证，不能当作端到端认证验收。

这些缺口按方法列在 ledger 中；同一命名空间的其他方法没有因此自动判为已验收。

## 本轮第二次静态复核

本轮检查了此前集中在 APIFull 中的礼物、社区、秘密聊天、举报、内联机器人、Bot / Mini App、付款、Stars 订阅、通话及其他方法，共把 183 个方法从“仅确认路由”改列为“静态确认不完整”。结论来自 handler 实现和已有测试源码；没有运行测试、调用线上或容器服务、也没有连接数据库。已有的往返测试只证明对应的本地记录能读回，不证明跨用户投递、权限、账务或外部服务工作。

- **礼物、兑换码与合集：** `payments.getStarGifts`、`saveStarGift`、`convertStarGift`、`getStarGiftUpgradePreview`、`upgradeStarGift`、`transferStarGift`、`getUniqueStarGift`、`getSavedStarGifts`、`getSavedStarGift`、`getStarGiftWithdrawalUrl`、`toggleChatStarGiftNotifications`、`toggleStarGiftsPinnedToTop`、`getResaleStarGifts`、`updateStarGiftPrice`、`getUniqueStarGiftValueInfo`、`checkCanSendGift`、`getStarGiftAuctionState`、`getStarGiftAuctionAcquiredGifts`、`getStarGiftActiveAuctions`、`resolveStarGiftOffer`、`sendStarGiftOffer`、`getStarGiftUpgradeAttributes`、`getCraftStarGifts`、`craftStarGift`；`payments.checkGiftCode`、`applyGiftCode`；`payments.createStarGiftCollection`、`updateStarGiftCollection`、`reorderStarGiftCollections`、`deleteStarGiftCollection`、`getStarGiftCollections`。礼物只用每用户一个文本/slug 模拟，合集只有一个标题；目录、库存、所有权、拍卖、兑换权益、合集条目与排序都没有业务实现。测试只验证保存 slug 或标题后本地可读回。
- **社区与秘密聊天：** `communities.create`、`togglePeerLink`、`getJoinedCommunities`、`toggleCommunityCollapsedInDialogs`、`getPeerLinkRequests`、`togglePeerLinkRequestApproval`、`toggleAllPeerLinkRequestApproval`、`toggleParticipantBanned`、`getParticipantJoinedChats`；`messages.requestEncryption`、`acceptEncryption`、`discardEncryption`、`setEncryptedTyping`、`readEncryptedHistory`、`sendEncrypted`、`sendEncryptedFile`、`sendEncryptedService`、`receivedQueue`。社区只写标题或原始请求、生成合成 Chat，部分查询固定空；秘密聊天只存调用者本地 ID/DH 字段，发送返回空加密结果且接收队列固定为空。`messages.getDhConfig` 及 `messages.uploadEncryptedFile` 不在这组缺口中。
- **举报与内联机器人：** `account.reportPeer`、`account.reportProfilePhoto`、`messages.reportSpam`、`messages.report`、`messages.reportEncryptedSpam`、`messages.reportMusicListen`、`channels.reportSpam` 只写本地举报日志；`messages.reportReadMetrics` 只更新 APIFull 本地 views。`messages.getInlineBotResults` 不执行 bot query；`setInlineBotResults` 只按 query ID 缓存；`sendInlineBotResult`、`editInlineBotMessage`、`sendBotRequestedPeer` 只存请求；callback 读写没有关联等待中的查询；`setBotGuestChatResult` 从 query ID 合成返回 ID。
- **事实核查、快捷回复与 Business Bot：** `messages.editFactCheck`、`deleteFactCheck`、`getFactCheck` 只按调用用户在 APIFull 本地读写，不确认消息存在或编辑权限，也不通知消息服务。`messages.getQuickReplies` 和 `getQuickReplyMessages` 通过 `MessageGetUserMessage` 读取真实消息对象；快捷回复关联仍是 APIFull 本地状态，但 `sendQuickReplyMessages` 已通过 `MsgSendMessageV2` 写入真实消息，`deleteQuickReplyMessages` 已返回 typed `Updates` 并删除关联。`account.updateConnectedBot` / `getConnectedBots` 仅有本地连接配置，未接到业务消息投递；`getBotBusinessConnection` 固定空，暂停/禁用设置只覆盖单条不被消费的 JSON。`ephemeral.reportMessage` 和 `getCallbackAnswer` 也只写入/读回每用户一条 note，没有举报或 callback 查询处理。
- **AI 文本、支持信息与广告：** `messages.summarizeText`、`translateRichMessage`、`composeRichMessageWithAI` 都返回原文或原 rich-text，没有摘要、翻译或生成服务。`messages.setTyping` 已补齐用户、普通群和 APIFull 频道的成员校验与 `UpdateChannelUserTyping` 推送；跨用户隔离会话已通过，生产双用户投递和重启持久性仍未验收。`help.getSupport`、`getSupportName`、`getInviteText` 固定空；`getAppUpdate` 不查询更新源而始终返回无更新。赞助消息的 `contacts.getSponsoredPeers`、`messages.getSponsoredMessages` 固定空，`account.toggleSponsoredMessages` 和 `channels.restrictSponsoredMessages` 只写本地 flag，view/click/report 只记本地事件，不接广告库存、分析或审核服务。
- **Passkey：** `account.initPasskeyRegistration`、`registerPasskey` 只处理挑战并存 credential ID，没有校验 attestation 或保存公钥；`getPasskeys`、`deletePasskey` 只管理这份本地 ID 列表。`auth.initPasskeyLogin` 使用空凭证白名单和 `localhost` RP；`auth.finishPasskeyLogin` 不验证 assertion，消费挑战后固定返回 `AUTH_BYTES_INVALID`，不会创建授权会话。
- **媒体与搜索：** `messages.getDocumentByHash` 校验参数后固定返回 `DocumentEmpty`，`help.getCdnConfig` 固定报 `CDN_METHOD_INVALID`，`messages.receivedMessages` 校验认证、请求构造器和 `max_id` 后在缺少回执 provider 时返回 `METHOD_NOT_IMPL`。`contacts.getTopPeers` 用用户 ID 合成排序分数且跳过非联系人分类；`messages.getRecentLocations` 把读取失败伪装成空结果并没有回填频道；`getSearchResultsPositions` 把当前页索引当全局位置；`channels.checkSearchPostsFlood` 永远给固定免费额度，不执行限流。
- **频道成员与导出：** `channels.getParticipants` / `getParticipant` 只把本地频道创建者当成员，无法列出普通成员或真实管理员。`account.initTakeoutSession` / `finishTakeoutSession` 只开关本地 session，不控制导出；`messages.getSplitRanges` 与 `channels.getLeftChannels` 在认证后 fail-closed 为 `METHOD_NOT_IMPL`，未实现 takeout 分区或退出频道历史，也不再伪造空结果。
- **Bot 与 Mini App：** `bots.createBot` 的标准创建和父 Bot deeplink 创建现在写入 User 服务 registry；deeplink 事务检查父 Bot 的 access hash 与默认关闭的 `bot_can_manage_bots`。该能力只允许运维通过受控数据库操作授予，客户端没有授予入口。`getAdminedBots` 按 `creator_user_id` 返回，`checkUsername` 查询全局用户名索引，`exportBotToken` 校验创建者并支持轮换；这些路径仍缺少完整 DB/session 验收。`getAccessSettings` 固定空，`editAccessSettings`、`setJoinChatResults`、两种默认管理员权限及 `setCustomVerification` 只保存请求或描述；`getBotRecommendations`、`getPopularAppBots` 固定空。`bots.sendCustomRequest` 回显输入 JSON，`answerWebhookJSONQuery` 与 `help.setBotUpdatesStatus` 只保存请求。`messages.requestWebView`、`requestSimpleWebView`、`requestAppWebView`、`requestMainWebView`、`requestChatJoinWebView`、`prolongWebView`、`sendWebViewResultMessage`、`sendWebViewData`、`getBotApp`，以及 `bots.invokeWebViewCustomMethod`、`checkDownloadFileParams`、`requestWebViewButton`、`getRequestedWebViewButton` 都是 URL 回显、本地记录或固定空值，未建立已授权的 WebView 会话或 Bot 调用链。常规 Bot 命令和菜单按钮本地读写不在此缺口列表。
- **付款、Stars 与预付 Giveaway：** `payments.getPaymentForm`、`sendPaymentForm`、`assignAppStoreTransaction`、`assignPlayMarketTransaction` 对有效凭证均返回 `PAYMENT_UNSUPPORTED`；收据不校验，Premium/权益不发放。`getPaymentReceipt` 只读本地合成记录，`validateRequestedInfo`、`getSavedInfo`、`clearSavedInfo` 在鉴权和必要的请求形状校验后返回 `METHOD_NOT_IMPL`，不再读写本地支付信息。`getBankCardData` 固定空。`payments.getStarsTopupOptions` 和 `getStarsGiftOptions` 固定空；`sendStarsForm` 只扣内部 Stars 余额并记本地流水；`refundStarsCharge` 对未知 charge 以 0 金额成功返回。Stars 订阅四个读写方法仅维护每用户一个本地订阅记录，没有续费或权益生命周期。`getPremiumGiftCodeOptions`、`getGiveawayInfo`、`launchPrepaidGiveaway`、`getStarsGiveawayOptions` 只保存一个 giveaway ID 并据此合成字段。
- **一对一 VoIP 与群通话：** `messages.deletePhoneCallHistory` 只存请求并返回 0 条受影响记录；`phone.requestCall`、`acceptCall`、`confirmCall`、`receivedCall`、`discardCall` 使用本地合成 ID / 状态，没有呼叫对端路由或信令投递；`setCallRating`、`saveCallDebug`、`sendSignalingData`、`saveCallLog` 只存请求。群/会议创建、加入/离开、参与者读取等方法只操作 APIFull 本地成员记录，没有通话控制面和跨客户端更新；流媒体 URL/频道只由本地参与者及 relay 配置构造，`phone.getGroupCallStars` 硬编码 0 星并把参与者当 donor。已在台账标为缺口的方法包括 `phone.createGroupCall`、`joinGroupCall`、`leaveGroupCall`、`getGroupCall`、`getGroupParticipants`、`checkGroupCall`、`getGroupCallJoinAs`、`getGroupCallStreamChannels`、`getGroupCallStreamRtmpUrl`、`createConferenceCall`、`deleteConferenceCallParticipants`、`getGroupCallStars`；现有已标缺口的邀请、录制、设置、屏幕共享、广播及通话消息方法继续保留原记录。

## 本轮第三次静态复核

本轮仍只读源码，没有运行测试、调用服务或连接数据库。源码明确显示下列 158 个路由方法只有本地模拟、固定空结果/固定零值，或没有执行协议要求的业务副作用，因此从“仅确认路由”改列为“静态确认不完整”。“不完整”不表示每个方法都完全空实现；具体证据在台账逐项列出。

| 功能 | 方法 |
| --- | --- |
| 限时动态 | `stories.canSendStory`、`sendStory`、`editStory`、`deleteStories`、`togglePinned`、`getAllStories`、`getPinnedStories`、`getStoriesArchive`、`getStoriesByID`、`toggleAllStoriesHidden`、`readStories`、`incrementStoryViews`、`getStoryViewsList`、`getStoriesViews`、`exportStoryLink`、`report`、`activateStealthMode`、`sendReaction`、`getPeerStories`、`getAllReadPeerStories`、`getPeerMaxIDs`、`getChatsToSend`、`togglePeerStoriesHidden`、`getStoryReactionsList`、`togglePinnedToTop`、`searchPosts`、`createAlbum`、`updateAlbum`、`reorderAlbums`、`deleteAlbum`、`getAlbums`、`getAlbumStories`、`startLive`。实现只维护调用者本地的合成记录，没有媒体存储、跨用户故事读取/投递或实时通话服务。 |
| 贴纸集和贴纸目录 | `messages.getStickers`、`getAllStickers`、`getStickerSet`、`installStickerSet`、`uninstallStickerSet`、`reorderStickerSets`、`getFeaturedStickers`、`readFeaturedStickers`、`getRecentStickers`、`saveRecentSticker`、`clearRecentStickers`、`getArchivedStickers`、`getMaskStickers`、`getAttachedStickers`、`getFavedStickers`、`faveSticker`、`searchStickerSets`、`toggleStickerSets`、`getOldFeaturedStickers`、`searchEmojiStickerSets`、`getMyStickers`、`searchStickers`；`stickers.createStickerSet`、`removeStickerFromSet`、`changeStickerPosition`、`addStickerToSet`、`setStickerSetThumb`、`checkShortName`、`suggestShortName`、`changeSticker`、`renameStickerSet`、`deleteStickerSet`、`replaceSticker`。目录/媒体是合成或固定空值，多数写方法只存请求、不改变读取路径。 |
| 自定义表情和关键词 | `account.getDefaultProfilePhotoEmojis`、`getDefaultGroupPhotoEmojis`；`messages.getCustomEmojiDocuments`、`getEmojiStickers`、`getFeaturedEmojiStickers`、`searchCustomEmoji`、`getEmojiKeywords`、`getEmojiKeywordsDifference`、`getEmojiURL`、`getEmojiGroups`、`getEmojiStatusGroups`、`getEmojiProfilePhotoGroups`、`getEmojiStickerGroups`。没有完整服务端表情目录；关键词只从当前用户自建贴纸取值，`getEmojiKeywordsLanguages` 目前只声明关键词接口真实可服务的 `en` 基线，URL 和其它目录接口仍为空或缺少权威媒体目录。 |
| 主题 | `account.uploadTheme`、`createTheme`、`updateTheme`、`saveTheme`、`installTheme`、`getTheme`、`getThemes`、`getChatThemes`、`getUniqueGiftChatThemes`、`messages.setChatTheme`。上传通过 DFS；create/update 仅接受调用者自己上传且已持久化的真实 Document，并保存主题元数据及可表示设置；全局主题目录和 chat-theme/unique-gift provider 仍未接入。 |
| 语言包 | `langpack.getLangPack`、`getStrings`、`getDifference`、`getLanguages`、`getLanguage`。翻译值回显请求 key/code，语言列表硬编码，差分版本不递增。 |
| SMS Jobs | `smsjobs.isEligibleToJoin`、`join`、`leave`、`updateSettings`、`getStatus`、`getSmsJob`、`finishJob`。只有本地标记/计数，没有任务分配或短信服务。 |
| Boosts | `channels.setBoostsToUnblockRestrictions`；`premium.getBoostsList`、`getMyBoosts`、`applyBoost`、`getBoostsStatus`、`getUserBoosts`。Boost 存为单个本地记录，数量与 ID 合成，没有扣除库存或修改频道状态。 |
| 游戏分数 | `messages.setGameScore`、`setInlineGameScore`、`getGameHighScores`、`getInlineGameHighScores`、`getEmojiGameInfo`。分数表按请求中的 peer/message ID 本地存储，没有验证游戏消息/权限；emoji game 忽略目标参数。 |
| 壁纸 | `account.getWallPapers`、`getWallPaper`、`uploadWallPaper`、`saveWallPaper`、`installWallPaper`、`resetWallPapers`、`getMultiWallPapers`、`messages.setChatWallPaper`。saved 列表仍是本地引用；上传的真实 DFS `Document` 现在保存到调用者自己的上传目录，`getWallPaper`/`getMultiWallPapers`/保存安装可读回该文档，但没有全局壁纸目录；聊天背景方法已写入 Dialog 的 peer 状态。 |
| 建议、深链和 Bot 信息 | `messages.getSuggestedDialogFilters`（固定空列表）；`help.getDeepLinkInfo`、`messages.startBot`、`help.getRecentMeUrls`（无后端数据源，现明确返回 `METHOD_NOT_IMPL`）；`help.dismissSuggestion`（固定成功、不记录）；`bots.getBotInfo`（忽略 bot/lang_code，总是读当前用户本地 Bot）。 |
| Premium 商店门面 | `help.getPremiumPromo`（硬编码“此服务器不提供 Premium”，无套餐）；`payments.canPurchaseStore`（忽略 purpose，只看本地 Premium 标记，没有商店/provider 检查）。 |
| 表情状态和颜色 | `account.updateEmojiStatus`、`getDefaultEmojiStatuses`、`getRecentEmojiStatuses`、`clearRecentEmojiStatuses`、`getChannelDefaultEmojiStatuses`、`getChannelRestrictedStatusEmojis`、`getCollectibleEmojiStatuses`；`account.updateColor`、`getDefaultBackgroundEmojis`、`channels.updateEmojiStatus`；`bots.updateUserEmojiStatus`、`toggleUserEmojiStatusPermission`。状态/颜色目录和跨用户同步仍缺失；recent 接口复用当前状态记录，clear 会把当前状态清空。`channels.updateColor` 已单独接入 canonical 频道资料，仍只有隔离数据库证据。 |
| 历史导入和讨论线程 | `messages.checkHistoryImport`、`initHistoryImport`、`startHistoryImport`、`checkHistoryImportPeer`（只记本地文件名/导入元数据，未导入消息）；`uploadImportedMedia` 已对带媒体请求校验导入会话和目标 peer，并通过 DFS 返回真实照片/文档，但最终导入消息投递仍未接入；`messages.getReplies`、`getDiscussionMessage`（合成线程/讨论消息，不读取真实回复或关联讨论群）；`messages.toggleSuggestedPostApproval`（只存单条 note，不更新频道待审批状态）；`payments.exportInvoice`（有效输入仍固定 `PAYMENT_UNSUPPORTED`）。 |
| 其他 | `account.getTmpPassword`（始终返回空令牌）；`messages.setBotShippingResults`、`setBotPrecheckoutResults`（认证并校验非零 `query_id` 后返回 `METHOD_NOT_IMPL`；没有待处理付款查询状态或消费响应的流程）；`fragment.getCollectibleInfo`（仅回显 slug）；`messages.hideChatJoinRequest`（当前只支持群组，不支持频道申请）；`account.updateUsername`（写入错误被吞掉，跨存储无回滚）；`stats.loadAsyncGraph`（空 token 返回 `TOKEN_EMPTY`，非空 token 返回 `METHOD_NOT_IMPL`，不再回显 token）；`aicompose.getToneExample`（忽略 tone，固定空文本）。 |

第三次复核时的台账分布为 451 项静态缺口和 323 项仅确认路由；此后新增的缺口和修复记录见下方第四次复核。

## 本轮第四次静态复核与修复

- `methods.json` 与 `LAYER229_METHOD_LEDGER.csv` 均为 813 个唯一方法，逐项集合完全匹配。第四次复核时台账有 463 项明确的静态功能缺口、311 项只有路由证据；本轮修复后为 460 项和 314 项。其余 39 项是隔离验收/部分验收、session 手动分发待验收或传输包装。仅有路由不代表已实现，仍缺方法级业务验收。
- 新确认的 10 项静态缺口已写入台账：`auth.exportAuthorization`、`auth.dropTempAuthKeys`、`users.suggestBirthday`、`messages.getWebPagePreview`、`messages.getWebPage`、`messages.readSavedHistory`、`updates.getChannelDifference`、`upload.getCdnFile`、`upload.reuploadCdnFile`、`upload.getCdnFileHashes`。授权导出返回的字节只是 DC 编号文本；临时密钥没有撤销路径；生日建议只有发送方到目标用户的本地记录，没有接收与处理流程；网页预览回显输入生成合成对象；已保存历史没有已读游标；频道差分缺少 PTS 事件日志，CDN 方法没有 CDN 服务。
- 联系人路径已改为读取真实联系人 ID 并计算 hash；按手机号删除现在传播数据库/缓存错误，只删除注册联系人和当前用户的已保存号码。缺少按导入者隔离删除未注册号码的 RPC，因此重置不会再调用影响所有用户的全局按号码删除。
- 入群申请按邀请链接限定群、链接和待处理用户。链接批处理分页读取该链接的申请并比较同链接处理前后状态；更新通知仍使用全群待处理列表。单条处理和全群申请查询传播存储错误。读取链接申请要求链接管理权限，可能严于群内 `CanInviteUsers`；权限不足时会报错并停止。`messages.hideChatJoinRequest` 仍不支持频道申请，邀请导入者的 `q` 搜索仍未实现。
- 文件夹与聊天列表现在使用请求中的 filter ID 查询置顶对话、校验成员并把删除文件夹中的对话移回主文件夹；聊天列表加入会复用或分配本地 filter ID，筛选器成员与已选择并加入的 peer 一致。跨 dialog 服务、KV filter 和 chatlist 状态的多个写入仍非原子，失败时可能部分完成。
- `updates.getChannelDifference` 现在明确返回 `METHOD_NOT_IMPL`，避免伪造空差分；由于没有可重放的频道 PTS 日志，该方法仍未实现。
- 本轮只做源码审查、格式检查和定向构建；没有运行测试、调用线上/容器服务或连接数据库。隔离功能验收和 36 个 Layer 229 构造器缺口仍需单独处理。

## 本轮修复

- `account.setAccountTTL` 现在把用户服务的写入错误返回给客户端，不再将持久化失败报告为成功。账号与联系人包构建通过；还没有用真实用户数据做有效 TTL 的写入和读回，因此从静态缺口转为未做功能验收。
- `contacts.getBlocked` 当时已使用 50 条上限；底层 `user.getBlockedList` SQL 按请求 `offset` 分页并传播 DAO 错误，也会回填群对象及频道插件可提供的频道对象。当时尚缺带总数的分页响应；这一点已在第十四次复核补齐。无频道插件时仍无法回填频道实体；该阶段只做构建检查，尚无真实数据验收。
- `contacts.block` / `contacts.unblock` 现在会传播用户资料读取和屏蔽状态写入错误，不再在服务端写入失败时返回成功。contacts 包构建通过，尚无真实屏蔽/取消屏蔽往返。
- `messages.sendMessage`、`messages.sendMedia` 和 `messages.sendMultiMedia` 现在处理结构化 `InputReplyToMessage.reply_to_peer_id`：发送前验证目标 peer 的 access hash/成员关系及源消息归属，收件端只对同一会话映射本地消息 ID，跨 peer 回复保留原 peer 和 ID。Story 回复因 messages 服务没有权威的存在性/可见性查询而在写入前返回 `METHOD_NOT_IMPL`；定时发送和 APIFull channelview 路径也不接受带回复的消息。此次未运行测试，未做真实发送或收件端读回，不能计为功能验收。
- `payments.refundStarsCharge` 现在按用户隔离本地 charge 记录，并对未知或损坏的 charge 返回 `PAYMENT_CHARGE_INVALID`，避免虚报退款成功。APIFull core 包构建通过；本地钱包模拟仍未对接真实支付提供方，也未做有效退款往返验收。`payments.sendStarsForm` 仍是本地余额模拟。
- `contacts.deleteContacts` 拒绝空、自身和不支持的 `InputUser`，传播用户读取及逐个删除错误；`InputUserFromMessage` 仍未支持。contacts 包构建通过，尚无有效删除及读回验收。
- `messages.getPeerSettings` 传播 peer 设置、用户和群读取错误，并回填可用用户/基础群对象。频道对象仍依赖 `Dialogs Plugin`，默认启动配置没有注入该插件；dialogs 包构建和 diff 检查通过，尚无真实 peer 往返验收。
- `account.getNotifySettings` 检查目标用户及基础群成员；频道查询通过可选 `NotificationPlugin`，没有插件时失败关闭。默认 BFF 注册传入 nil，因此频道分支仍不可用；聚合目标和用户 access hash 未逐项校验，尚无功能验收。
- `users.getSavedMusic` 现在使用完整列表计算 saved-ringtone 风格 hash，支持 not-modified、offset/limit，并返回总数。userchannelprofiles 包构建和 diff 检查通过；尚无真实用户数据验收。
- `account.getRecentEmojiStatuses` 与 `account.clearRecentEmojiStatuses` 已拆分 recent 列表与当前 emoji 状态；首次读取旧记录时只迁移一次，清除 recent 不再清掉 active status。APIFull core 包构建通过；这些状态仍是调用者本地数据，未同步到共享用户资料或其它客户端。
- 认证代理在收到“不新增或运行测试”的约束前短暂启动过一次测试：account 的临时测试因构造器名称错误未能编译，contacts 包通过；其新增测试文件随后已删除。其余本轮改动仅有定向构建，没有运行测试、连接服务或访问数据库。

## 本轮第五次复核

- 当前 ledger 与 `../telegram-api/methods.json` 的 813 个方法逐项匹配。最新计数为 459 项静态确认缺口、315 项仅确认路由；另有 13 项隔离库验收、3 项隔离库部分验收、8 项隔离组件验收、2 项组件部分验收、2 项 session 手动分发待验收、11 个传输包装。`users.getSavedMusic` 本轮从静态缺口转为等待真实数据验收。
- 本轮三个独立修复已完成定向构建和 diff 检查，没有运行测试，也没有连接服务或数据库。除上面逐项列出的实现边界外，剩余方法仍按 ledger 状态处理；有路由或合成返回不代表业务验收。

## 本轮第六次复核：生产验收推进

- 用户将目标明确为 813 个方法逐项真实业务流程验收。当前审计把“隔离 core/DAO 测试”与“完整 BFF/session 端到端验收”分开计数；全量端到端通过数仍是 0/813。台账新增两个验收列，标记了每项证据的层级，避免把组件测试、路由命中或返回类型算作生产通过。
- **频道成员路径：** `channels.joinChannel`、`leaveChannel`、`inviteToChannel`、`getParticipants`、`getParticipant` 已有按频道和用户持久化的成员状态与基础权限检查。`TestChannelsMemberLifecycleRoundTrip` 在 `teamgram_audit` 隔离库覆盖加入/邀请/退出、读回、重复操作、成员与非成员、错误 access hash、私有频道和创建者限制；UserClient 使用 stub，没有经过真实 BFF/session 或用户服务。邀请仍未接隐私审核与跨客户端更新，参与者过滤器和用户资料对象不完整，故五项只标为 `ISOLATED_CORE_ONLY` / 静态不完整。
- **已保存历史已读游标：** `messages.readSavedHistory` 现在校验调用者、Saved Messages parent、peer 和非负游标，经 dialog gRPC 单调写入 `saved_dialogs.read_max_id`；仅更新已有条目。BFF 请求映射测试和隔离 MySQL 上的 bufconn gRPC→DAO 读回、游标不回退及拒绝分支通过。完整部署的 BFF→biz/session 链路未验收，新增迁移只应用于审计库，因此标为 `PARTIAL_ISOLATED_DB_ONLY`。
- **已屏蔽列表分页：** 本轮修复 `user.getBlockedList` DAO 原先忽略 offset 的问题，并让 SQL 读取错误返回上层。只运行 `user/internal/core` 与 DAO 定向构建和 diff 检查；没有真实用户服务 RPC、权限、分页读回或重启测试，`contacts.getBlocked` 仍未验收。该阶段的 blockedSlice 总数缺口已在第十四次复核补齐；默认频道实体插件仍缺失。
- **原生频道消息历史：** 复核确认当前 BFF 把所有频道历史路由到 APIFull 专用表；该表路径忽略 access hash/成员权限且不覆盖完整 TL 分页。原生 message service 没有对应频道历史存储和权限检查 RPC，不能安全地用通用消息 RPC 作为回退。需要先提供一条在权威频道存储上校验身份/访问哈希/成员权限并处理完整分页的 canonical RPC；当前风险是原生消息误报为空及 APIFull/native 数据域混读。该缺口尚未修复或验收。
- **运行环境边界：** 本轮只读确认 `backend-backend-1` 中 BFF 和业务进程正在运行，且默认服务环境不适合作为写入验收目标；没有向该容器发送写请求或重启它。`layer229-audit-mysql` 的 `teamgram_audit` 是隔离库，现有已确认 schema 主要为 APIFull 测试表及本轮游标表；它不等同于一套完整隔离的 Teamgram 服务栈。未执行完整 BFF/session 部署验收，也没有运行生产流量或负载/故障恢复测试。
- 本轮 `contacts.getBlocked` 改动的定向构建通过；其他构建与测试证据见对应小节/台账。本轮未触碰默认 `teamgram` 数据库、未重启现有服务，也未调用破坏性业务方法。

## 接下来

按 `LAYER229_METHOD_LEDGER.csv` 逐项补业务输入、真实副作用、读回、权限边界和持久性证据。优先处理频道成员/访问控制与原生消息数据路径，再处理群通话、付费与礼物账务；每项都在隔离审计库或隔离服务上验证。破坏性方法只做隔离数据的实现验证，不对默认业务数据执行。


## 本轮第七次复核：隔离数据库回读

- `contacts.getBlocked` 的 `TestUserPeerBlocksSelectListHonorsOffset` 已在 `layer229-audit-mysql` 的 `teamgram_audit` 执行通过，插入隔离用户的 4 条屏蔽记录后验证 `offset=1, limit=2` 读回 peer ID `[2, 3]`，并清理测试行。DAO 测试只验证 SQL 分页；新增 BFF 单测验证 offset、50 条上限，以及频道实体无法 hydration 时失败关闭。仍未覆盖完整 session→BFF→用户服务 RPC、默认 Plugin 注入、重启持久性或权限流程；台账状态为 `PARTIAL_ISOLATED_DB_ONLY`。
- 复核 `messages.getSavedHistory` 后更正旧台账描述：handler 已使用 `channelview.ChatsByID` 回填 APIFull 频道对象，但不包含仅存在于原生 channel service 的频道；没有完整混合消息历史读回测试，因此仍保留为有缺口且未验收。
- `channels.getMessages` 在读取 APIFull 本地消息前，现在校验输入频道的 access hash 和持久化成员关系。`TestChannelsGetMessagesAuthorization` 在 `teamgram_audit` 验证创建者/成员读到已有消息、非成员被拒绝和错误 access hash 被拒绝。原生频道消息仍没有权威读取路径，因此仅为隔离存储部分验收。
- `channels.updateColor` 不再写入调用者的个人 accent key；它现在校验频道 access hash，将 peer color/profile color 写入隔离频道资料并回填 Layer 229 Chat。`TestChannelsUpdateColorRoundTrip` 在 `teamgram_audit` 通过，覆盖持久化读回、Layer 229 编码、个人颜色不变及非创建者拒绝。APIFull 模型目前只有创建者关系，因此更广泛的频道管理员权限和完整 BFF/session 流程仍未验收。
- `auth.importAuthorization` 不再伪造带 `UserEmpty` 的授权成功；当前返回 `METHOD_NOT_IMPL` 并由 `TestAuthImportAuthorizationDoesNotFabricateSuccess` 覆盖。要实现它仍需 authsession 提供有期限、绑定目标 DC/来源 session 的传递 token，以及目标端原子消费、密钥绑定和用户/2FA 流程，因此仍是实现阻塞，不计作已实现。
- `messages.sendMedia` 现在把 `InputReplyToStory` 映射为 `MessageReplyStoryHeader`，并交给现有消息服务持久化路径。`TestMessageReplyToStoryHeaderFromUser`、`FromPeer` 和 `RequiresOwner` 通过。它们只验证 TL header 构造，未调用消息服务或 session/BFF；缺少 story owner 时 handler 会在发送前拒绝，因此台账记为部分组件验收。
- `messages.readMessageContents` 现在根据权威 `MessageBox` 把用户/群消息按 peer 分组，并在任何写操作前验证整批记录；原先误用第一条消息的 peer 会漏处理其它 peer。`TestGroupReadMessageContentsByResolvedPeer` 和 `TestGroupReadMessageContentsRejectsUnsupportedPeerBeforeMutation` 通过。频道消息仍不支持，按 peer 分组的多个写入不具备跨服务原子性，未做完整 BFF/session 验收。
- `auth.signUp` 新增本地名称校验；`TestNormalizeSignupName` 通过，覆盖首名必填、可选姓氏、裁去首尾空白、非法 UTF-8/控制字符和超过 64 个 Unicode 字符的拒绝。该测试不调用短信、建号或 session 路径；手机号 flood protection 仍未实现，因此台账只记为 `PARTIAL_COMPONENT_ONLY`。
- `account.updateUsername` 的 `TestUpdateUserUsernameAuditDatabase` 已在同一隔离 MySQL 容器的独立 `teamgram_username_audit` 数据库实际运行通过，覆盖资料和用户名索引原子更新/读回、重复用户名拒绝且不改写、资料写入失败回滚索引。BFF 单测 `TestAccountUpdateUsernameRejectsInvalidFormatBeforeRPC`、`TestAccountUpdateUsernamePropagatesWriteFailure`、`TestAccountUpdateUsernameRejectsFalseBackendReply` 通过。用户名修改的同步通知错误仍被忽略，且没有完整 session 或多会话同步验收，因此只记录为部分隔离库证据。
- 再次从 `../telegram-api/methods.json` 的所有分类展开后比较，清单和台账均为 813 个唯一方法，缺失/多余项均为 0。当前验收状态分布为：446 项仍有已知实现缺口、317 项只有路由证据、13 项隔离库验收、8 项部分隔离库验收、8 项隔离组件验收、5 项隔离 core 验收、5 项部分组件验收、11 项传输包装；完整 BFF/session 生产验收仍为 0/813。覆盖集合一致和局部隔离证据都不代表生产全链路通过。
- 新增 auth.importAuthorization 的失败关闭单测和 channels.getMessages 的隔离访问控制测试；前者只证明不会再回报合成成功，后者只证明 APIFull 本地数据路径的权限，不构成真实授权导入或原生频道验收。
- 汇总定向回归已通过：屏蔽列表 MySQL DAO 分页、用户名 MySQL 事务与 BFF 校验、auth.signUp 名称校验、消息已读内容 peer 分组，以及 APIFull 频道颜色隔离 MySQL 往返；contacts/user core 包无测试文件，但编译通过。
- 以上验证只连接两个隔离审计库；没有访问默认 `teamgram` 数据库、运行中的 BFF/session、或重启现有服务。

## 本轮第八次复核：频道读取权限与授权传递

- `channels.readHistory` 现在先校验输入频道的 access hash 和调用者成员关系，再写入每用户已读游标。`TestChannelsReadHistoryRoundTrip` 在 `teamgram_audit` 覆盖创建者与成员读回、非成员和错误 access hash 拒绝、拒绝时游标不变，以及游标不倒退。此路径只覆盖 APIFull 本地频道数据；原生频道读取和完整 BFF/session 链路仍未验收，因此只记为部分隔离库验收。
- `messages.getHistory` 的本地频道分支现在用原始 `InputPeerChannel` 校验 access hash 和持久化成员，再读本地消息。`TestHistoryForInputPeerChecksHashAndMembership` 在 `teamgram_audit` 验证成员读回消息、错误 hash 与非成员被拒绝。测试覆盖 BFF 调用的 channelview helper，不覆盖 session/BFF 网络链路；原生频道存储、PTS 和完整分页仍未实现，因此只记为部分隔离库验收。
- `auth.exportAuthorization` 已移除“用户 ID + 十进制 DC ID”伪造的导出凭证，现在在缺少跨 DC 授权传递能力时返回 `AUTHORIZATION_TRANSFER_UNAVAILABLE`。`TestAuthExportAuthorizationFailsWithoutTransferPrimitive` 验证不返回授权对象并返回 406。跨 DC 导出/导入仍需 authsession 提供有期限且绑定目标 DC 的 token 签发、验证和消费能力；两个方法仍未实现。
- APIFull 的 `account.confirmPasswordEmail` 和 `account.resendPasswordEmail` 已接入权威 challenge store 与邮件 provider；无邮箱、空验证码、未签发验证码和无邮件 transport 都返回明确错误且不改写确认状态。两个定向 fail-closed 测试仍覆盖无 provider 场景；真实邮件 endpoint、Redis 持久化和完整确认/重发链路尚未验收。
- `messages.receivedMessages` 现在先校验认证、nil 请求和负 `max_id`，再在没有回执持久化或 push 通知取消 provider 时返回 `METHOD_NOT_IMPL`，不再返回空成功向量。`TestMessagesReceivedMessagesRejectsInvalidInput` 和 `TestMessagesReceivedMessagesFailsClosedWithoutProvider` 通过。方法功能仍未实现。
- 无 AI backend 的 5 个路径不再伪造结果：APIFull 的 `messages.composeMessageWithAI` / `aicompose.getToneExample` 返回 `METHOD_NOT_IMPL`，不再写请求 JSON 或生成空文本；消息 BFF 的 `messages.summarizeText`、`messages.translateRichMessage`、`messages.composeRichMessageWithAI` 不再把原文当作总结、翻译或 AI 输出。5 个定向测试均通过；实际 AI、摘要和翻译功能仍未接入。
- `payments.sendStarsForm` 不再按请求 invoice 扣减本地模拟余额并返回成功。由于没有 canonical Stars form/charge provider，现在返回 `METHOD_NOT_IMPL` 且不变更余额、Stars 交易、支付台账或 charge 记录。两个定向隔离库测试通过；真实支付仍未实现。
- 这一轮有局部回读和错误路径证据，但没有连接运行中的 BFF/session 或默认业务库，也没有将任何方法记为生产端到端通过。

### Provider retry policy evidence

- `pkg/code/delivery.go` 的可配置 HTTP provider 使用 `ProviderTimeoutSeconds` 和 `ProviderRetryCount`；每次请求重新构造 body/context，只有网络错误或 HTTP 5xx 消耗重试次数，HTTP 4xx/3xx、空 endpoint、无 client 和 context 取消立即失败。定向 `pkg/code/delivery_test.go` 覆盖 5xx 恢复、网络错误恢复、重试上限和 4xx/3xx 不重试；`go test ./pkg/code/... ./app/bff/authorization/internal/core` 与 `go test -race ./pkg/code/...` 通过。没有真实 SMS/email endpoint 或凭据，不能升级生产验收状态。

## 本轮第九次复核：QR、密码恢复、消息与 Stars 缺口

- `auth.exportLoginToken` 现在校验 `api_id`/`api_hash` 格式、保存并执行 `except_ids`，Redis 创建与 New token 轮换通过 Lua CAS，Accepted/Success 响应会确认 QR auth key 已绑定到目标用户。当前没有可信应用凭证注册表，所以 `api_hash` 仅做格式检查；多 DC 迁移仍未实现。QR core/DAO 定向测试和 race 测试通过，但未跑完整 session→BFF→Redis/authsession 流程。
- `auth.importLoginToken` 现在处理同 DC 的 token 查询和 Accepted 成功状态，并在返回成功或删除缓存前确认 auth key 绑定用户及密码状态。标准多 DC 导入/迁移和完整 session 授权仍未验收。Redis accept 与 authsession bind 不共享事务；绑定失败时 token 会停留在 Accepted 直到 TTL，必须重新签发。
- `auth.acceptLoginToken` 先用 Redis CAS claim token generation，再绑定 auth key；并发 accept/rotate 测试保证旧 token 不会覆盖 Accepted 状态。Redis 与 authsession 故障恢复没有跨服务事务保障，因此仍是部分实现。
- `messages.getMessagesViews` 现在对空 ID 保留 `INPUT_REQUEST_INVALID`，其他请求在无权威阅读计数时返回 `METHOD_NOT_IMPL`，不再合成 0/空 views 或按真实频道返回 enterprise 拦截。定向 peer/空 ID 测试通过；阅读数服务仍未实现。
- `payments.refundStarsCharge` 在没有退款 provider 时返回 `METHOD_NOT_IMPL`，不再更改本地模拟余额或报告退款成功。`TestStarsRefundUnavailableMySQL` 在 `teamgram_audit` 验证余额、流水表、历史、支付台账和 charge 记录均不变。真实支付退款仍未实现。
- `payments.sendPaymentForm` 现在先检查配置的 HTTP provider，再以精确币种/金额、交易号、非空收据和 `verified=true` 完成 durable payment ledger settlement；provider 缺失或响应不匹配会失败关闭并拒绝请求。没有 Premium/Stars entitlement 写入、退款 provider 或真实支付往返验收，因此仍是部分组件实现。
- `payments.getPaymentForm` 现在在配置 provider 时发送带 `operation=get_form` 的请求，只接受 `verified=true`、非空发票和正数 `form_id`，再交给 MTProto 编码；provider 缺失、HTTP 失败或表单结构无效均失败关闭。provider 仍需提供权威商品和支付表单，未做外部 provider 或真实会话业务流验收。
- `phone.declineConferenceCallInvite` 现在从调用者自己的消息盒读取并验证 `messageActionConferenceCall`，校验 `InputGroupCall` access hash 后持久化移除参与者和状态；跨客户端邀请通知与生产媒体信令仍未接入。
- `auth.requestPasswordRecovery`、`auth.checkRecoveryPassword`、`auth.recoverPassword` 在没有邮件投递能力时不再生成或接受本地伪造验证码；请求路径撤销旧本地码，有已确认邮箱时返回 `METHOD_NOT_IMPL`，没有恢复邮箱仍返回 `PASSWORD_RECOVERY_NA`。内存存储定向测试覆盖不签发和旧码不可用。没有真实邮件验证码流程。
- `payments.getStarsTopupOptions`、`getStarsGiftOptions`、`getStarsRevenueStats`、`getStarsRevenueWithdrawalUrl`、`getStarsRevenueAdsAccountUrl` 不再以空套餐、零收入或空 URL 冒充成功。仓库没有权威商品目录、收入源、提款 provider 或广告账户服务，因此均在鉴权后返回 `METHOD_NOT_IMPL`。五方法定向测试覆盖 nil 结果和错误；真实 Stars 商业流程仍未实现。
- `chatlists.getChatlistUpdates` 现在从已知本地邀请读取缺失 peer 并回填频道实体；未知邀请来源失败关闭，过期 slug 返回无效邀请。`joinChatlistUpdates` 会先确认所选 peer 属于邀请，再做文件夹/加入状态写入；越权 peer 不会触发写入。`getLeaveChatlistSuggestions` 和 `messages.getSuggestedDialogFilters` 没有权威推荐源，改为 `METHOD_NOT_IMPL`。相关定向测试在 `teamgram_audit` 通过，完整 session/有效加入流程仍未验收。
- `auth.initPasskeyLogin` 和 `auth.finishPasskeyLogin` 因缺少受信 RP 配置、凭证选择器、公钥存储和 WebAuthn assertion verifier，现不再生成无验证用途的 localhost challenge/options，也不消耗 challenge 或创建登录态；格式合法时明确返回 `METHOD_NOT_IMPL`。本次只做静态审查和格式检查，没有运行测试。
- `ephemeral.sendMessage`、`ephemeral.deleteMessage`、`ephemeral.reportMessage`、`ephemeral.getCallbackAnswer` 没有权威临时消息、举报或 bot 回调服务；现在均鉴权后返回 `METHOD_NOT_IMPL`，不再写 personal note 或伪造 Updates/Bool/ReportResult。send/delete 手动分发 wrapper 不再为失败请求打开存储。新增了无读写断言，但本批没有运行测试。
- QR、消息阅读数与退款的局部定向测试不构成生产端到端验收；全量生产验收仍为 0/813。

## 本轮第十次复核：搜索、授权撤销与举报假成功

- 当时逐项比较 Layer 229 `methods.json` 与台账：813 个方法，台账 813 行且名称集合完全一致。生产端到端验收仍为 0/813。静态缺口最多的命名空间是 messages 137、payments 58、account 56、phone 40、stories 33、channels 30、bots 21、auth 16。
- 认证路径现在检查 `authsession.bindAuthKeyUser` 的错误、空响应和零 hash，覆盖 `auth.signIn`、`auth.signUp`、`auth.importBotAuthorization` 和 future-auth-token 登录；logout/reset 路径也会传播 authsession DAO 的查询、删除和缓存错误，logout 还会传播 future-token 写入失败。API 凭证只校验正数 ID 和 32 位十六进制 hash；仓库仍没有可信应用凭证注册表，不能据此认证凭证真实性。
- `messages.searchGlobal` 现在只接受后端能表达的首屏、无筛选请求；缺少认证或 message/user/chat provider 时会在访问前失败关闭。文件夹、筛选、日期、非空游标和其它不支持的字段会明确返回 `METHOD_NOT_IMPL`；搜索 DAO 错误、用户/群对象 hydration 错误会传播，缺少本地频道对象时也明确失败。消息服务仍没有 peer-local 分页游标或权威原生频道 hydration。`messages.searchSentMedia` 会校验认证、filter 构造器和 limit 边界，bot 或 malformed 请求会失败关闭；合法请求通过 `MessageSearchByMediaType` 的 `PEER_UNKNOWN/0` 分支查询所有会话，并由 `SelectSentByMediaTypeWithCB` 返回真实消息。BFF 定向测试通过，消息服务有隔离 MySQL 审计测试；本轮未配置 `MESSAGE_AUDIT_MYSQL_DSN`，也未做 live session/生产流量验收。`channels.searchPosts` 仍因缺少符合请求语义的后端而失败关闭；`messages.search` 和 `messages.getSearchCounters` 现在传播后端错误，但仍只支持部分过滤器。
- APIFull 的 peer、profile photo、消息、频道举报和 read-metrics/music-listen 提交方法现在鉴权后返回 `METHOD_NOT_IMPL`，不再写入无人消费的本地 report blob 或 view counter。Layer 229 的 `stories.report` 也不再修改个人故事存储或伪造 `ReportResult`。Stars 状态、交易记录和按 ID 查询同样不再返回本地模拟数据，缺少权威 Stars 账本时返回 `METHOD_NOT_IMPL`。这些改动只证明假成功已被移除，不代表相应业务功能已经实现。
- 为与新行为一致，更新了举报和 story 举报的旧断言；本轮运行了 `messages.searchSentMedia` 的 BFF 定向测试，消息服务 MySQL 审计测试因未配置 `MESSAGE_AUDIT_MYSQL_DSN` 跳过，未部署或重启进程。完整的短信登录、受信 API 凭证校验、Stories/Stars/举报提供方、原生频道搜索与分页仍是待补缺口。


## 本轮第十一次复核：授权撤销、群邀请、搜索与联系人令牌

- 重新比较 `../telegram-api/methods.json` 与台账：813 个唯一方法，缺失/多余均为 0。当前审计状态列为 466 项已知静态缺口、298 项仅确认路由、13 项隔离库验收、13 项部分隔离库验收、8 项隔离组件验收、2 项部分组件验收、2 项 session 手动分发待验收及 11 项传输包装。生产验收列仍为 0/813 完整端到端通过；457 项被实现缺口阻塞，297 项未验收，其余只有隔离库、组件或 core 层证据。
- `messages.search` 的消息 provider 现在在 DAO 前拒绝空查询、非法 peer 和负 limit，并传播 hashtag/全文 DAO 错误，同时使用规范化 offset/limit；voice 与 round-video 过滤器映射到消息服务支持的 media type 9 和 11。BFF 映射测试及生成 gRPC 客户端→消息服务→DAO→隔离 MySQL 测试通过，并确认数据库错误会传播。mentions、geo、contacts 过滤器及完整 query/pagination 语义仍不支持，因此该方法仍列为有实现缺口。
- `messages.exportChatInvite` 与 `messages.getExportedChatInvites` 的底层 chat gRPC 流程现在从调用元数据校验发起者身份、群成员和邀请权限。隔离 MySQL 测试覆盖导出、持久化字段、列表读回、非成员/普通成员/跨管理员权限拒绝以及缺失身份拒绝；fixture 行已清理。BFF/session、事件投递和实体 hydration 仍未验收。
- `auth.resetAuthorizations` 和 `account.resetAuthorization` 的底层 authsession gRPC→MySQL 流程已通过隔离审计库测试。覆盖保留当前 session、用户隔离、按 hash 撤销、墓碑状态、缓存失效，并确认返回永久 auth key ID。测试发现并修正了把已撤销永久 key ID 错映射成临时 key ID 的问题。BFF/session 更新投递与进程重启未验收。
- `contacts.exportContactToken` 现在生成 24 小时随机 token URL，并把 owner/expiry 存入 APIFull 持久层；`contacts.importContactToken` 校验缺失、未知、过期、自身及失效用户 token，解析用户、写入联系人关系并返回实际 User。核心 MySQL 往返、拒绝分支和写失败传播测试通过；UserClient 使用替身，临时审计 schema 在测试后移除，因此只记部分隔离库证据。
- 本轮只使用 `layer229-audit-mysql` 中的隔离审计 schema 和 bufconn gRPC 测试服务器；临时数据库账号按库授权并在结束后删除。没有连接 `teamgram` 默认业务库、向运行中的 backend 服务发 RPC、重启服务或执行破坏性 Layer 229 方法。

## 本轮第十二次复核：消息编辑、在线状态与联系人添加

- 重新从 `../telegram-api/methods.json` 展开 813 个方法，与台账逐项对比：813 个唯一名称完全匹配，无缺失或多余。生产级 BFF/session→业务服务→持久化/provider→读回和权限验证仍为 **0/813**。
- `messages.getMessageEditData` 的频道分支读取 APIFull 存储的真实频道文本消息，验证 access hash、成员关系、创建者权限和消息存在性；caption 目前为 false。`TestHistoryAndEditDataCheckInputPeerAuthorization` 在隔离 `teamgram_audit` 通过。2026-10-07 r20 生产只读 session 调用以 `InputPeerChannel`/`id=7` 返回 `messages.MessageEditData(caption=false)`，且 `messages.getHistory` 与 MySQL 对照确认 TL ID 映射、调用前后消息行相同（写入 0；见上方记录）。生产证据只覆盖 APIFull 文本消息，原生频道消息存储与媒体 caption 仍未验收。
- `account.updateStatus` 现在先持久化 last-seen，再通过 `SyncUpdatesNotMe` 把 `updateUserStatus` 发给当前账号其它会话，并排除当前永久授权密钥。替身客户端测试验证调用顺序、字段和持久化/推送失败传播。联系人扇出仍缺少按联系人、屏蔽状态、状态隐私规则和群例外计算收件人的 User 服务接口，所以仍是部分实现。
- `contacts.addContact` 对安全的 `InputUserFromMessage` 子集增加真实消息查找和上下文核验：私聊只接受对端用户，群/频道只接受消息作者；再调用联系人写入服务。定向测试使用消息与用户服务替身，覆盖成功、Peer/作者不匹配和读取失败。其它消息来源类型未支持，且没有真实联系人持久化往返。
- `account.updateEmojiStatus` 现在调用已有的 user-service RPC 更新共享用户状态，保存近期列表，并发送 `updateUserEmojiStatus` 到当前账号其它会话；收藏状态因现有用户存储只能表示 document ID/expiry 而明确返回 `METHOD_NOT_IMPL`。定向测试在隔离 APIFull 测试库启动的包环境下通过，但 UserClient/SyncClient 使用替身，所以不算真实服务或 MySQL 回读验收。联系人推送仍未实现。
- `account.getDefaultEmojiStatuses`、`getChannelDefaultEmojiStatuses`、`getChannelRestrictedStatusEmojis`、`getCollectibleEmojiStatuses`、`account.getDefaultBackgroundEmojis`、`channels.updateEmojiStatus`、`bots.updateUserEmojiStatus` 和 `bots.toggleUserEmojiStatusPermission` 缺少权威目录、状态或权限后端，已改为 `METHOD_NOT_IMPL`。这避免把当前选择冒充目录、空列表当成已支持，或把本地请求 JSON 当成有效写入。`TestEmojiStatusMethodsFailClosedWithoutCatalogOrBackend` 和更新后的背景 emoji 断言通过隔离 APIFull 测试包运行。
- 以上四项均没有向运行中的 Teamgram 服务发送 RPC、连接默认 `teamgram` 数据库或部署进程。本轮复核后台账为 462 项静态缺口、298 项仅确认路由、14 项部分隔离库验收、13 项隔离库验收、8 项隔离组件验收、5 项部分组件验收、2 项 session 手动分发待验收和 11 个传输包装；完整生产验收仍为 0/813。

## 本轮第十三次复核：联系人来源与通知实体完整性

- `contacts.search` 现在传播联系人列表、用户名搜索、姓名搜索和用户实体 hydration 的错误；用户结果按 `user.getContactIdList` 的权威 ID 集合分类为 `my_results`，不再使用候选用户的反向联系人关系。频道命中会校验 resolver 返回的类型、ID、重复项和完整集合；生产 contacts BFF 仍传 `Plugin=nil`，群聊结果因现有 `chat.search` 忽略 `self_id` 而失败关闭。底层 `user.search` / `user.searchUsername` 也不再吞掉 DAO 错误。定向 core/service 测试通过；没有完整 session 或真实搜索数据验收。
- `contacts.deleteContacts` 支持安全的 `InputUserFromMessage` 子集：读取调用者的消息副本，校验来源 peer、消息 ID 和作者后才执行联系人删除；消息不匹配、查询失败、自身用户和其它不支持构造器均拒绝。定向测试使用 Message/User/Sync 替身通过；多联系人删除没有跨服务原子性，也未做真实持久化回读。
- `account.getNotifyExceptions` 不再静默省略频道对象，也不再忽略用户或基础群 hydration 错误。现在会在缺少频道插件、返回集合缺失/重复/越权实体或实体形状错误时失败关闭，并只在所有实体成功 hydration 后组装 Updates。定向 core 测试通过；生产 notification BFF 没有频道查询插件，完整 session→BFF 验收仍未进行。
- `messages.getPeerSettings` 现在先验证 peer，再读取个性化设置；用户 access hash、用户/群对象完整性、群成员状态和频道 access hash 都会核对。用户/群/设置服务错误会返回，频道插件缺失或返回实体不匹配时失败关闭。定向测试覆盖合法用户/基础群/频道和错误分支；生产 dialogs BFF 未注入频道插件，方法仍有实际可用性缺口。
- `messages.setTyping` 现在先验证认证元数据，再验证 InputPeerUser 的实体和 access hash、群组发起者成员资格与活跃接收者，并传播 user/chat/sync 错误；nil Action 会被拒绝。因 dialogs 无权威频道成员查询/广播路径，频道请求明确返回 `METHOD_NOT_IMPL`。定向测试通过，但没有验证完整 session 路径或实际在线接收会话；群组逐个同步推送在中途失败时可能已有部分接收者收到。
- `account.getPrivacy` / `account.setPrivacy` 现在传播规则与 user/basic-group hydration 错误，并在 set 前完整解析对象、检查持久化 BoolTrue 后再同步更新。没有频道查询后端时，带频道的规则会失败关闭。同步失败发生在规则持久化之后，返回错误会明确说明规则已保存但通知失败；不能回滚两个服务间的部分提交。定向替身测试通过，未做 session 或真实多会话流程验收。
- 本轮没有向运行中的 Teamgram 服务发送 RPC，没有连接默认 `teamgram` 数据库，没有重启进程，也没有执行破坏性联系人操作。重新比较 `../telegram-api/methods.json` 与台账后，813 个唯一方法仍完全匹配；当前台账计数为 461 项明确静态缺口、298 项仅确认路由、14 项部分隔离库验收、13 项隔离库验收、8 项隔离组件验收、6 项部分组件验收、2 项 session 手动分发待验收和 11 个传输包装。生产验收仍为 0/813，另有 452 项被实现缺口阻塞、297 项未验收、18 项部分隔离库、13 项隔离库、8 项隔离组件、9 项部分组件、5 项隔离 core 和 11 项传输包装。

## 本轮第十四次复核：已屏蔽列表分页总数

- `user.getBlockedList` 现在通过用户服务查询 `user_peer_blocks` 中该用户未删除记录总数，并在现有 `Vector_PeerBlocked` 的新增 `total_count` 字段中返回；`contacts.getBlocked` 使用现有生成的 `contacts.blockedSlice`，即使 offset 页为空也携带该总数。返回页仍限制为最多 50 条。
- `TestContactsGetBlockedPassesOffsetAndCapsLimit` 通过 protobuf marshal/unmarshal 验证 count 字段可传输，并验证 BFF 返回 `contacts.blockedSlice` 与同一总数；`TestContactsGetBlockedFailsClosedWithoutChannelHydration` 仍覆盖缺少频道插件时不返回部分成功。两个定向 BFF 测试通过。用户服务 core 与 DAO 包通过 compile-only 检查。
- DAO 测试已增加隔离行数 count 断言，但本轮环境未设置 `APIFULL_MYSQL_DSN`，因此没有执行该 SQL count 测试；此前隔离库验证只覆盖 offset 分页。仍未验证完整 session→BFF→用户服务调用，也未解决生产 contacts 服务不注入频道插件的问题。台账保持 `PARTIAL_ISOLATED_DB_ONLY`，不代表生产验收。
- 更新后台账仍与 `../telegram-api/methods.json` 的 813 个唯一方法完全匹配：460 项静态缺口、298 项仅确认路由、14 项部分隔离库验收、13 项隔离库验收、7 项部分组件验收、8 项隔离组件验收、2 项 session 手动分发待验收和 11 项传输包装。生产验收仍为 0/813；另有 451 项被实现缺口阻塞、297 项未验收、18 项部分隔离库证据、13 项隔离库证据、10 项部分组件证据、8 项隔离组件证据、5 项隔离 core 证据和 11 项传输包装。

## 本轮第十五次复核：占位成功与错误传播

- `messages.getMessages` 现在拒绝未认证请求和非正消息 ID，传播消息、用户与基础群 RPC/实体读取错误；读取结果涉及频道而当前 BFF 无频道 resolver 时返回 `METHOD_NOT_IMPL`，不再忽略错误或静默省略频道实体。`go build ./app/bff/messages/internal/core` 通过；没有运行测试或访问服务/数据库。该方法改列为静态缺口，完整业务链路仍未验收。
- `account.getBotBusinessConnection` 校验调用者和 connection ID 后返回 `METHOD_NOT_IMPL`；APIFull 只保存按用户关联的 connected-bot 偏好，不存 connection ID，也没有权威业务连接查询服务。
- `communities.getParticipantJoinedChats` 校验调用者、社区频道和参与者后返回 `METHOD_NOT_IMPL`；现有 chat RPC 只提供基础群成员关系，没有社区到群组映射、创建者/加入者分类或所需实体聚合。
- 两个 `stats.getMessagePublicForwards` 构造器验证频道访问哈希、创建者与本地源消息；`stats.getStoryPublicForwards` 验证调用者自己的已存储 Story。三者都因没有权威转发记录或反向索引而返回 `METHOD_NOT_IMPL`，不再返回伪造的空转发列表。
- `contacts.search` 现在通过 `chat.search` 查询当前用户仍在群内的基础群（`chat_participants.state=0`），并校验完整群实体；查询与 hydration 错误会传播。`chat.search` 的非零 offset 语义没有定义，当前对该参数失败关闭；联系人搜索使用首屏 offset=0。同一隔离审计用例中的 `ChatSearch` 通过 bufconn 调用 Chat gRPC，在 `layer229-audit-mysql/teamgram_audit` 命中当前成员所在的临时群组，排除已退出群（`state=1`）和非成员，并在测试清理临时数据。生产 contacts 服务仍以 `Plugin=nil` 初始化，频道结果失败关闭；没有完整 session→BFF→Chat gRPC 验收。
- `TestChatInviteExportReadbackOverIsolatedRPC` 中的 `ChatSearch` 子路径已通过隔离 `teamgram_audit` 数据库上的 bufconn Chat gRPC 验证，临时群组数据由测试清理。没有连接运行中的 Teamgram 服务或默认 `teamgram` 数据库，也没有验证完整 session→BFF 链路。完整生产验收仍为 0/813。

## 本轮第十六次复核：移除无后端方法的伪成功

- `channels.readMessageContents` 之前把整条请求写进 APIFull 通用 KV 后返回 `BoolTrue`，并未更新阅读状态。现有 `messages.readMessageContents` 与 msg 内容读取路径仅处理私聊和基础群；APIFull 频道消息表也只保存文本，没有媒体未读状态。该方法现在先校验认证，再返回 `METHOD_NOT_IMPL`。`TestChannelsReadMessageContentsFailsClosedWithoutChannelContentStore` 验证了已认证请求不会伪成功。
- `account.getTmpPassword` 之前返回全零 `account.tmpPassword`。仓库只有 2FA SRP/KDF 和一次性挑战验证，没有临时支付密码签发、存储、过期及消费链路；支付表单也不接真实 provider。现已改成认证后返回 `METHOD_NOT_IMPL`。`TestAccountGetTmpPasswordFailsClosed` 覆盖此拒绝分支。
- `communities.getParticipantJoinedChats` 已检查认证、社区频道和用户 peer；仓库没有社区到基础群的映射、创建者/加入者分类或完整实体 hydration provider，因此继续失败关闭为 `METHOD_NOT_IMPL`，没有合成空列表。
- 上述 APIFull core 聚焦用例由 `go test ./app/bff/apifull/internal/core -run '^(TestChannelsUnauthed|TestChannelsReadMessageContentsFailsClosedWithoutChannelContentStore|TestPaymentsUnauthed|TestAccountGetTmpPasswordFailsClosed)$' -count=1` 通过；Go 测试进程只连接隔离容器的 `teamgram_audit`，未调用运行服务或默认 `teamgram` 库。该验证证明错误路径，不代表方法业务功能通过。`contacts.search` 的群搜索另有隔离 MySQL + bufconn Chat gRPC 回读证据，但频道搜索仍因生产 contacts 服务 `Plugin=nil` 而失败关闭。
- 清点后台账仍为 813 个方法且与 `methods.json` 一致：461 项静态缺口、297 项仅确认路由、13 项隔离库验收、14 项部分隔离库验收、8 项隔离组件验收、7 项部分组件验收、2 项 session 手动分发待验收和 11 个传输包装。生产验收为 0/813；451 项受实现缺口阻塞、296 项未验收、19 项部分隔离库证据、13 项隔离库证据、8 项隔离组件证据、10 项部分组件证据、5 项隔离 core 证据及 11 项传输包装。隔离拒绝测试不计为功能通过。

## 本轮第十七次复核：频道公开链接与联系人错误边界

- `channels.exportMessageLink` 不再生成空链接。它先验证调用者、频道 access hash 和源消息，再通过 `user.getChannelUsername` 读取同一频道的权威 active username，并拒绝无 username、错误 peer、分组链接和线程链接。`TestChannelsExportMessageLinkPublicMessageRoundTrip` 及负向用例在隔离 `teamgram_audit` 上通过，覆盖 Layer 229 编码、缺失消息和错误 access hash；频道消息和频道行由测试清理。UserClient 使用测试替身，完整 session/BFF 和真实 username 服务未验收，因此台账标为 `PARTIAL_ISOLATED_DB_ONLY`。
- `contacts.getContactIDs` 现在要求认证，传播 `user.getContactList` 错误，拒绝 nil/非法联系人和超出协议 `int` 范围的 ID，按协议排序并计算 hash；`contacts.getStatuses` 传播联系人和 last-seen RPC 错误，空联系人不发空查询，nil 响应/条目失败关闭。两组 focused core 测试通过，均使用内存 UserClient stub，没有生产服务或持久化验收，因此台账标为 `PARTIAL_COMPONENT_ONLY`。
- APIFull 测试入口现在严格拒绝非 `tcp://127.0.0.1:13306/teamgram_audit` 的 `APIFULL_MYSQL_DSN`，避免测试误连默认库。重新解析 `methods.json` 与台账后仍为 813 个唯一方法，名称和构造器 ID 缺失/多余/不匹配均为 0。当前台账计数为 460 项静态缺口、295 项仅确认路由、15 项部分隔离库验收、13 项隔离库验收、9 项部分组件验收、8 项隔离组件验收、2 项 session 手动分发待验收和 11 个传输包装；生产验收仍为 0/813，另有 450 项被实现缺口阻塞、294 项未验收、20 项部分隔离库证据、13 项隔离库证据、12 项部分组件证据、8 项隔离组件证据、5 项隔离 core 证据及 11 项传输包装。

## 本轮第十八次复核：频道删除事务与联系人实体回填

- `channels.deleteChannel` 已从“保存请求后返回空 Updates”改为真实事务删除。它锁定频道创建者，校验 access hash 和所有权，在同一事务中删除频道、成员、消息、隐藏消息、消息序列、阅读状态和频道群通话；成功后清除旧 KV 记录。`TestChannelsDeleteChannelOwnerTransactionRoundTrip` 在隔离 `teamgram_audit` 通过，验证非创建者和错误 access hash 不改数据，创建者删除后所有相关表计数为 0，并验证 Layer 229 Updates 编码。完整 session/BFF、更新投递和重启验收仍未做，因此台账标为 `PARTIAL_ISOLATED_DB_ONLY`。
- `contacts.getContacts` 不再吞掉联系人或用户回填错误。现在要求认证，空联系人直接返回空协议向量，非空列表会校验联系人和用户实体完整性并传播依赖错误；focused core 测试覆盖实体回填、空列表、错误响应、nil 行、缺失用户、认证和 nil 请求。测试使用 UserClient stub，没有真实用户服务或 session 持久化验收，因此台账标为 `PARTIAL_COMPONENT_ONLY`。
- `user.getChannelUsername` 现在传播 UsernameDAO 的数据库错误；不存在或空用户名返回 `USERNAME_NOT_OCCUPIED`，不再把后端故障伪装成空结果。取消上下文的 DAO 错误传播测试通过，未连接数据库；该内部服务修复被 `channels.exportMessageLink` 的权威用户名读取使用。
- 本轮台账仍与 `methods.json` 保持 813 个唯一方法和构造器 ID 完全匹配：459 项静态缺口、294 项仅确认路由、16 项部分隔离库验收、13 项隔离库验收、10 项部分组件验收、8 项隔离组件验收、2 项 session 手动分发待验收和 11 个传输包装。生产验收仍为 0/813，另有 449 项被实现缺口阻塞、293 项未验收、21 项部分隔离库证据、13 项隔离库证据、13 项部分组件证据、8 项隔离组件证据、5 项隔离 core 证据及 11 项传输包装。

## 本轮第十九次复核：频道管理员权限与成员实体

- `channels.editAdmin` 不再保存请求后返回空更新。APIFull 现在在隔离 MySQL 中为频道成员保存管理员权限和 rank，要求频道 access hash、目标用户真实存在、目标已经入群，并允许创建者或持有 `add_admins` 的管理员修改；目标为创建者、非成员、越权调用和过长 rank 都会拒绝。空权限对象会原子清除管理员权限和 rank。
- 权限变更会返回 `updateChannelParticipant`，带有前后 participant、actor、频道实体和目标用户；`channels.getParticipants` 的 admins filter 与 `channels.getParticipant` 会读回管理员 participant constructor、rights 和 rank。
- APIFull 频道成员结果在配置了 UserClient 时通过 `user.getMutableUsersV2` 回填完整用户实体；UserClient 缺失的隔离 core 测试仍保留 ID-only fallback，避免把没有实体服务的局部测试误报成生产成功。
- `TestChannelsEditAdminRoundTrip` 在 `layer229-audit-mysql` 的 `teamgram_audit` 通过，覆盖创建者升权、被授予 `add_admins` 的管理员继续授权、admins 列表和单成员读回、实体 hydration、降权、越权、非成员拒绝以及频道清理。测试使用 UserClient stub，没有连接运行中的 session/BFF，也没有默认 `teamgram` 数据库验收。
- 台账与 `../telegram-api/methods.json` 仍为 813 个唯一方法，缺失/多余/构造器 ID 不匹配均为 0。当前审计状态为 456 项静态缺口、294 项仅路由、19 项部分隔离库验收、13 项隔离库验收、10 项部分组件验收、8 项隔离组件验收、2 项 session 分发待验收和 11 项传输包装；生产验收仍为 0/813，448 项被实现缺口阻塞、293 项未验收、24 项部分隔离库、13 项隔离库、13 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装。

## 本轮第二十次复核：频道位置和频道写入访问校验

- `channels.editLocation` 不再把请求写入通用 KV 后伪造成功。频道表现在持久化纬度、经度和地址；`inputGeoPointEmpty` 会清空三者，坐标范围、地址长度、频道 access hash 和创建者权限都会在写入前校验。`channels.getFullChannel` 从同一行读回 `channelLocation`/`channelLocationEmpty`，并可按 Layer 229 编码。
- `channels.getFullChannel`、`channels.editTitle`、`channels.deleteMessages`、两个 `channels.deleteHistory` 构造器和频道设置更新现在统一验证 `InputChannel.access_hash`，避免错误 hash 读写 APIFull 本地频道。隐藏参与者频道的非成员列表请求仍返回空集合，单成员查询继续拒绝泄漏成员信息。
- `TestChannelsEditLocationRoundTrip`、`TestChannelMutationRequiresAccessHash`、`TestChannelSettingsRoundTrip` 和全频道定向测试在隔离 MySQL `teamgram_audit` 通过；覆盖位置读回、Layer 229 编码、清空、越权、坏 hash 以及拒绝写入不改变标题/设置/消息。完整 session→BFF、原生频道位置 provider 和运行服务验收仍未完成，因此台账只记 `PARTIAL_ISOLATED_DB_ONLY`。
- 以当前 CSV 重新计数：审计状态为 455 项静态缺口、292 项仅路由、22 项部分隔离库、13 项隔离库、10 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 447 项被实现缺口阻塞、291 项未验收、27 项部分隔离库、13 项隔离库、13 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装。813 个方法的完整生产端到端验收仍为 **0/813**。
- 当前生产级端到端验收仍为 **0/813**；本轮未连接默认 `teamgram` 数据库、未向运行中的 backend 发送 RPC、未重启进程，也未执行破坏性操作。

## 本轮第二十一次复核：共同群组、共同频道分页和错误传播

- `messages.getCommonChats` 的基础群路径现在按 `user_id` 显式匹配两份成员列表，不再依赖 RPC 返回顺序；空/重复用户响应不会误把其它用户的群组当成共同群。`max_id` 和 `limit` 会稳定分页，默认和上限均为 100，chat hydration 失败、空响应和缺失实体会返回错误而不是空成功。
- `messages.getCommonChats` 现在通过 APIFull 成员表补充共同频道，按频道 ID 去重并回填 `Channel`。真实隔离 Alice/Bob GramJS 调用返回了共同基础群和临时频道；`TestCommonChannelIDsIncludesActiveMembers`、common-chats handler 测试及带隔离 DSN 的三包测试均通过。原生频道仍没有统一 provider，因此生产验收仍保持未完成。
- 当前 CSV 重新计数为：453 项静态缺口、276 项仅路由、25 项部分隔离库、13 项隔离库、25 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 444 项实现缺口阻塞、277 项未验收、30 项部分隔离库、13 项隔离库、27 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装。完整生产端到端验收仍为 **0/813**。

## 本轮第二十二次复核：联系人导入、频道封禁和群详情

- `contacts.importContacts` 的 BFF 现在拒绝错误构造器、空白电话、重复电话和重复 `client_id`，保留用户服务结果与请求输入的顺序，并校验导入联系人和 User 实体的完整性。空 Vector 是无副作用空结果。UserCore 同样校验输入，按输入顺序处理联系人，并传播电话号码查询、联系人关系写入、缓存失效、用户 hydration 和未注册联系人写入错误。BFF focused tests 使用 UserClient stub 与内存保存号码存储；UserCore tests 只覆盖格式错误和空 Vector，故两侧只记组件部分证据，未连接真实 User 服务或 MySQL，也没有 session 链路。
- `messages.getFullChat` 现在通过现有 dialog RPC/DAO 读取置顶消息，回填 DialogExt 提供的 folder、theme emoticon 和 TTL，并传播关键 Chat/Dialog/User RPC 错误。意外 nil 响应和缺少实体会失败关闭。`HasScheduled`、`GroupcallDefaultJoinAs` 及部分邀请/通话元数据仍没有权威数据源。focused helper tests 与 BFF/Dialog core/gRPC 编译检查通过，没有实际 session、服务或数据库往返，所以生产状态仍被实现缺口阻塞。
- `channels.editBanned` 在 APIFull 隔离数据模型中持久化成员限制、封禁者和时间，验证频道 access hash、管理员 `ban_users` 权限和目标用户；禁止受委派管理员修改其他管理员，已过期限制会清除，kick/unban 会更新成员状态。隔离库 `TestChannelsEditBannedRoundTrip` 通过，覆盖返回 Updates 的 Layer 229 编码、限制读回、未知/删除用户、权限、坏 hash、管理员目标、过期限制和 kick/unban。该 API 路径尚未证明限制会被消息发送业务路径执行，因此只记 `PARTIAL_ISOLATED_DB_VERIFIED`，不算完整功能或生产验收。
- 本轮测试通过：contacts BFF 与 UserCore 两组 focused tests；Chats/Dialog core focused tests；`TestChannelsEditBannedRoundTrip` 使用容器 `layer229-audit-mysql` 的 `teamgram_audit`。核心相关包 compile-only 检查通过，`git diff --check` 通过。没有连接默认 `teamgram` 数据库、访问运行中的 BFF/session 或重启服务。
- 重新解析 `../telegram-api/methods.json` 并比较台账：813 个源方法、813 行台账、唯一名称各 813 个，missing/extra 均为 0。相对第二十一次复核，1 项从静态缺口转为部分隔离库证据，1 项从仅路由转为部分组件证据；当前静态缺口为 454、仅路由为 291、部分隔离库为 23、隔离库为 13、部分组件为 11、隔离组件为 8、session 分发待验收为 2、传输包装为 11。生产状态为 446 项被实现缺口阻塞、290 项未验收、28 项部分隔离库、13 项隔离库、14 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装；完整生产端到端仍为 **0/813**。`channels.editBanned` 是部分隔离库证据，`contacts.importContacts` 是部分组件证据，`messages.getFullChat` 仍有实现缺口。

## 本轮第二十三次复核：历史位置、联系人导入和失败关闭

- `messages.getRecentLocations` 的 user/self/basic-chat 路径现在向后分页读取，不局限于最初一页；筛选 geo、live geo 和 venue 后按 `limit` 返回，并传播历史读取、nil 页面、无进展分页及 user/chat 实体补全错误。频道路径先用现有历史读取校验 access hash 和成员身份；频道消息存储只保留文本，缺少权威媒体查询，因此合法频道请求明确返回 `METHOD_NOT_IMPL`，不伪造空位置列表。底层 backward offset-ID 查询错误现在从 MySQL DAO 传到 Message Core；这条历史路径的其它调用者也会收到错误，forward 和 offset-date 路径的 DAO 错误传播仍未覆盖。
- `contacts.importContacts` 现在为未注册号码批量计算 `PopularContact.importers`，来源是 `unregistered_contacts` 中 `imported=0` 的 distinct importer 数，联系人 upsert 保证重复请求不重复计数。UserCore 定向包测试通过；隔离 MySQL 用例因 `APIFULL_MYSQL_DSN` 未设置而跳过，因此仍只算组件证据。
- `channels.deleteParticipantHistory` 的 APIFull 本地模型增加事务删除、创建者/`delete_messages` 权限校验及本地 PTS 递增，并新增隔离 MySQL 用例。核心包 compile-only 检查通过；`127.0.0.1:13306/teamgram_audit` 拒绝所提供凭据（MySQL 1045），用例未执行。APIFull 当前发送路径只有创建者可写文本消息，普通成员作者历史及与生产消息路径的完整连接均未验收，仍保留实现缺口状态。
- `messages.getSearchResultsPositions` 现在校验必填字段后返回 `METHOD_NOT_IMPL`，因为现有搜索后端没有全局排名或精确总数，不再把页内下标伪装成全局位置。`users.suggestBirthday` 保留输入校验后失败关闭，不再保存无人读取的 JSON 或返回空 Updates。`messages.reportReaction` 保留认证后失败关闭，不再写入无人消费的本地 JSON；仓库没有审核队列或 moderation consumer。`account.reportPeer` 复核后确认此前已是认证后失败关闭，本轮无需修改。
- 本轮验证通过：`messages.getRecentLocations` handler 与历史 DAO/Core 定向测试、相关 MySQL DAO 测试和 gRPC service 编译；`contacts.importContacts` UserCore 包测试；`users.suggestBirthday` 定向及完整 core 包测试；`messages.reportReaction` 定向测试和 APIFull core 编译。`git diff --check` 通过。没有连接默认 `teamgram` 数据库、运行中的服务、部署或重启。
- `methods.json` 与台账仍是 813 个唯一方法，missing/extra 均为 0。当前审计状态：454 项静态缺口、290 项仅路由、23 项部分隔离库、13 项隔离库、12 项部分组件、8 项隔离组件、2 项 session 分发待验收、11 项传输包装。生产状态：446 项实现缺口阻塞、289 项未验收、28 项部分隔离库、13 项隔离库、15 项部分组件、8 项隔离组件、3 项隔离 core、11 项传输包装；完整生产端到端仍为 **0/813**。

## 本轮第二十四次复核：联系人重置、完整用户错误传播和音乐文档授权

- `contacts.resetSaved` 现在先调用 User 服务，再清空 BFF saved-phone 列表。User DAO 在同一事务中删除调用者的注册联系人和未注册号码记录、清除反向 mutual 标记及该调用者作为 importer 的关系，并失效受影响的联系人缓存。BFF 测试使用 UserClient stub；User core/DAO 编译通过，但事务 SQL 未在 MySQL 执行。BFF 本地列表和 User DB 不共用事务。
- `users.getFullUser` 现在传播 User、Chat、Dialog 查询错误，并对缺失的本人/目标用户及关键 nil 响应失败关闭。`chat.getUsersChatIdList` 也会透传 DAO 错误；隐私规则按真实基础群成员匹配，频道成员规则因没有权威数据源而返回 `METHOD_NOT_IMPL`。基础群成员、DAO 错误、unsupported channel rule、实体缺失、自查和 peer-settings 错误有 focused tests。常见群数量仍只计基础群，频道成员及若干可选 full-user 字段仍不完整。
- `users.getSavedMusicByID` 现在校验 InputUser 和 access hash，只返回目标用户保存列表中的文档，并检查 SAVED_MUSIC 隐私和文档 access hash。未知的 chat-participant 隐私规则及 nil 成功响应失败关闭；目标保存列表中的过期文档 ID 会被过滤。focused tests 使用 User/Media stubs，未做服务或持久化往返。
- 定向验证通过：联系人重置 BFF 测试、`users.getFullUser` 与 Chat core 测试、`userchannelprofiles` core 包测试；User core/DAO compile-only 检查通过；`git diff --check` 通过。未连接默认 `teamgram` 数据库、运行中的服务，没有部署或重启。
- 重新解析 `telegram-api/methods.json` 并比较台账：源清单、台账均为 813 个唯一方法，名称与构造器 ID 的 missing/extra/mismatch 均为 0。当前审计状态为 452 项静态缺口、289 项仅路由、23 项部分隔离库、13 项隔离库、15 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装。生产状态为 444 项实现缺口阻塞、288 项未验收、28 项部分隔离库、13 项隔离库、18 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装；完整生产端到端仍为 **0/813**。

## 本轮第二十五次复核：回复 peer 校验与无 provider 接口失败关闭

- `messages.sendMessage`、`messages.sendMedia` 和 `messages.sendMultiMedia` 对结构化 `InputReplyToMessage.reply_to_peer_id` 在发送前校验来源用户 access hash、基础群成员身份，或频道 access hash/成员及来源消息存在性；用户/群消息还需确认消息 ID 属于指定 peer。`msg` 收件 DAO 仅对同一会话执行原有本地消息 ID 映射，跨 peer 回复保留原 peer 和 ID。旧式 `reply_to_msg_id` 及省略 `reply_to_peer_id` 的同会话结构化回复仍没有 BFF 来源消息校验，channel peer 和定时发送中的带回复请求失败关闭。Story 回复因 messages 服务无权威 story 可见性查询，在 `deliverStored` 或消息写入前返回 `METHOD_NOT_IMPL`。没有运行测试或验证真实发送、收件端回读。
- `account.updateColor` 的普通 `PeerColor` 与旧字段现在调用 User 服务，并传递 `for_profile`、颜色和背景 emoji；collectible 颜色失败关闭。Peer color 查询仍来自内置三色列表，默认背景 emoji 清单没有 provider；没有做 User 服务持久化或 session 回读。
- 赞助消息开关、频道限制、查看、点击、举报及两个赞助内容查询现在对有效请求返回 `METHOD_NOT_IMPL`，不再写无人消费的本地标记，也不再将空赞助清单作为成功。仓库没有广告库存、偏好消费者、曝光/点击分析或举报后端。`account.sendVerifyEmailCode` 校验 email 和 purpose 后，在生成或缓存验证码前返回 `METHOD_NOT_IMPL`；当前没有邮件传输。
- 重新解析 Layer 229 清单与台账：813 个唯一方法全部匹配，缺失/多余/构造器 ID 不匹配均为 0。当前审计状态为 452 项静态缺口、289 项仅路由、23 项部分隔离库、13 项隔离库、15 项部分组件、8 项隔离组件、2 项 session 分发待验收及 11 项传输包装。生产状态为 444 项实现缺口阻塞、289 项未验收、28 项部分隔离库、13 项隔离库、17 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装；完整生产端到端仍为 **0/813**。
- `git diff --check` 通过；本轮没有运行测试、连接数据库或运行服务，没有部署或重启。

## 本轮第二十六次复核：未读提及与转发边界

- `messages.getUnreadMentions` 的基础群路径现在要求认证、非 bot、合法 peer、非负 limit 和正常群成员；消息 RPC、用户/群 hydration 和 SQL 查询错误都会透传，nil 响应或无效消息盒失败关闭。未读提及 DAO 将 `min_id` / `max_id` 作为 `user_message_box_id` 的互斥边界应用。频道未读提及仍返回 `CHANNEL_UNREAD_MENTIONS_UNSUPPORTED`，因为消息服务和 APIFull 频道存储都没有按用户维护的未读提及状态；本方法没有完整 session 或数据库回读验收。
- `messages.readMentions` 现在只支持基础群：BFF 校验请求者、peer、可选 `top_msg_id`、群实体和正常成员状态后，调用 `msg.readMentions` 的事务 provider，按可选上限清除当前用户群消息的 `mentioned` 标记、重算 dialog 未读 mention 数量，并返回真实 `pts/pts_count`。它不会推进整个会话的已读游标或清除媒体未读状态；频道、用户和消息来源 peer 仍明确返回 `METHOD_NOT_IMPL`/参数错误。
- `messages.forwardMessages` 在写入前验证认证、来源和目标 peer、access hash、成员关系，以及每个请求消息的 owner、peer 和 box/message ID 对应关系，并传播消息、隐私、联系人和 DAO RPC 错误；消息服务缺失或发送返回 nil Updates 时失败关闭。可降级的纯文本路径拒绝媒体、格式实体、回复、分组消息、转发限制和不支持的标志；定时转发与目标频道转发返回 `METHOD_NOT_IMPL`，因为现有存储路径不能保留转发内容和 `random_id` 幂等性。定向 BFF/message-service 测试通过，但没有真实 session 发送、收件端读回或隔离数据库验收。
- 重新解析 `../telegram-api/methods.json` 并与台账逐项比较：813 个唯一方法和 constructor ID 完全匹配，missing/extra/mismatch 均为 0。当前审计状态为 454 项静态缺口、287 项仅路由、23 项部分隔离库、13 项隔离库、15 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产验收仍为 **0/813**。
- 本轮只做格式与差异检查；未运行测试、连接数据库或服务，没有部署或重启。

## 本轮第二十七次复核：Bot 付款查询响应失败关闭

- `messages.setBotShippingResults` 和 `messages.setBotPrecheckoutResults` 现在保留认证检查，并拒绝 nil 请求或零 `query_id`；其余请求返回 `METHOD_NOT_IMPL`，不再将通用支付日志写入当作响应成功。
- 静态搜索仅找到这两个 APIFull 路由和处理器，没有 shipping/precheckout 查询的创建者、待处理查询存储或响应消费者。`recordPay` 按用户 ID 追加操作名，不保存 `query_id`、结果或待处理状态，不能完成付款查询响应。未连接服务或数据库；没有做完整 BFF/session 流程验收。
- 台账保持 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP`，两项证据更新为无状态源/消费者及合法输入失败关闭；本轮未运行测试或 build。

## 本轮第二十八次复核：outbox 阅读日期和清单一致性

- `messages.getOutboxReadDate` 现在校验认证、用户 peer/access hash、请求者拥有的发件消息、目标账号状态，以及双方的 `STATUS_TIMESTAMP` 隐私规则；只有隐私规则明确拒绝时才返回对应 403。损坏的隐私数据失败关闭，无法评估的高级规则返回 `METHOD_NOT_IMPL`。有效阅读日期使用 604800 秒客户端/TDLib 默认过期值；当前服务端没有动态 `pm_read_date_expire_period` 配置，因此仍列为静态不完整，且没有完整业务链路验收。
- 复核 `methods.json` 与 ledger 的名称和构造器 ID：813 个源方法、813 行唯一台账记录，missing/extra/mismatch 均为 0。发现旧生成接口 `stats.getBroadcastRevenueWithdrawalUrl` 不属于这份 Layer 229 清单，已从 813 方法台账排除；它不计入本轮 API 覆盖总数。
- 更新此前过时描述：`contacts.blockFromReplies` 仍在写入前返回 `METHOD_NOT_IMPL`；`messages.readDiscussion` 已保存单调递增的讨论已读游标，但没有更新推送；`stats.loadAsyncGraph` 对合法 token 失败关闭；Stars 收入/提现/广告账户 URL 不再返回伪造零值或空 URL。
- 本轮只做静态源码和台账核对；未运行测试/build，未连接服务或数据库，没有部署或重启。

## 本轮第二十九次复核：读取错误传播与联系人屏蔽确认

- `contacts.block` 现在要求 User 服务确认屏蔽写入，并传播 User RPC 与跨会话 Sync 错误，避免依赖失败时返回 `BoolTrue`。`TestContactsBlockFailsWhenUserServiceDoesNotPersist`、`TestContactsBlockPropagatesSyncFailureAfterPersistence` 和成功路径 focused 测试通过；这些测试使用服务替身，未验证真实 User/MySQL 写入或多会话投递。Sync 失败发生在屏蔽关系已持久化之后。
- `messages.getHistory` 现在传播 user/chat hydration 错误；`messages.getDialogs` 传播 Dialog、通知设置、置顶消息及 user/chat hydration 错误。两组 focused 错误路径测试及 `-race` 运行通过，未覆盖有效数据的完整 session/BFF 读回，也未覆盖成功但实体不完整的响应。
- `phone.saveDefaultGroupCallJoinAs` 现在校验请求与两个 InputPeer 的结构，并新增 peer 过滤及用户隔离读回测试。当前环境未设置 `APIFULL_MYSQL_DSN`；APIFull `go test -c` 编译通过，但未执行 `TestMain` 或该数据库测试。`accent_colors.go` 和 `channel_ad_revenue.go` 中三个现有编译错误已最小修复，供 APIFull 包编译验证。
- 重新解析 `../telegram-api/methods.json` 与台账：813 个唯一方法、813 行唯一记录，名称 missing/extra 与 constructor ID mismatch 均为 0。当前审计状态为 455 项已知静态缺口、284 项仅确认路由、23 项部分隔离库、13 项隔离库、17 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装。生产状态为 446 项实现缺口阻塞、285 项未验收、28 项部分隔离库、13 项隔离库、19 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装；完整生产端到端验收仍为 **0/813**。
- 本轮没有向运行中的 Teamgram 后端发送请求，没有连接默认数据库、部署或重启服务。隔离服务栈和完整 session 级验收 harness 仍缺失。

## 本轮第三十次复核：联系人确认与贴纸卸载读回

- `contacts.acceptContact` 现在传播 User lookup 与 `UserAddContact` 写入错误；`messages.editMessage` 传播 `MessageGetUserMessageList` 错误，不再把依赖故障报告成无效联系人或空消息。两个 focused stub 测试通过；联系人持久化、多会话投递以及消息编辑成功读回仍未验证。
- `messages.uninstallStickerSet` 现在按 set token 从调用者的安装列表移除记录，`messages.getAllStickers` 可读回其余集合。安装、卸载和读回回归用例已加入并带 fixture 清理；因 `APIFULL_MYSQL_DSN` 未配置，只编译了 APIFull 测试二进制，没有运行用例或连接数据库。共享贴纸目录及真实媒体变更仍不可用。

## 本轮第三十一次复核：取消屏蔽与保存对话读取错误

- `contacts.unblock` 现在要求 User 服务返回确认，并传播跨会话 Sync 错误；focused stub 测试覆盖失败确认、Sync 失败和成功。Sync 失败发生在取消屏蔽写入之后。
- `messages.getSavedDialogs` 现在传播 top-message 查询错误，避免返回没有对应消息的 dialogs；focused stub 测试通过。
- `messages.toggleStickerSets` 的 uninstall 子路径会批量移除本地安装 token；archive、unarchive 和混合 flags 在写入前返回 `METHOD_NOT_IMPL`，没有可执行操作时返回输入错误。回归用例覆盖卸载读回与失败关闭，但只编译、未运行，因为 APIFull 隔离 DSN 未配置。
- 最终重解析 Layer 229 清单与 ledger：813 个唯一方法、813 行唯一记录，missing/extra/constructor ID mismatch 均为 0，所有 CSV 行列宽和生产状态字段有效。当前审计状态为 455 项已知静态缺口、280 项仅确认路由、23 项部分隔离库、13 项隔离库、21 项部分组件、8 项隔离组件、2 项 session 分发待验收和 11 项传输包装。生产状态为 446 项实现缺口阻塞、281 项未验收、28 项部分隔离库、13 项隔离库、23 项部分组件、8 项隔离组件、3 项隔离 core 和 11 项传输包装；完整生产端到端验收仍为 **0/813**。
- 本轮只运行 focused stub 测试及 APIFull 测试二进制编译，没有调用运行中的服务、默认数据库或 session，没有部署或重启。

## 本轮第三十二次复核：十四个虚拟用户和真实 Web 业务流

- 隔离 MySQL fixture 提供 `isolated_test_01` 到 `isolated_test_14`，手机号为 `12025550101` 到 `12025550114`，固定测试码为 `12345`。fixture 只写入 `teamgram-web-isolated-*` 私有卷。
- `docker/web-isolated/verify-business-flows.sh` 通过真实 WebSocket → session → BFF 路径让十四个用户登录，覆盖联系人、屏蔽、私聊收发/搜索/已读/编辑/置顶/转发/草稿、未读对话、十四人群组、频道发送/历史/阅读数/搜索/差分/详情/成员及非管理员拒绝。重启隔离 `teamgram` 和 `gateway` 清理旧会话后再次通过：`business-flow acceptance passed: isolated-e2e-1790465921733-44768`。
- Web 真实浏览器验收位于 `telegram-tt-master/tests/playwright-real/isolated-web.spec.ts`，使用两个独立 Chromium context，验证 owner 搜索并选中 13 名成员、创建 14 人群组、发送消息、打开群详情检查成员列表，再由 member 登录并回读消息。`npm run test:playwright:real -- --reporter=line` 通过（46.4s）。
- `messages.getCommonChats` 的 Alice/Bob 真实 GramJS 回读同时返回共同基础群和 APIFull `Channel`；带 `root:audit-only@tcp(127.0.0.1:13306)/teamgram_audit` 的 channelview/domain/chats 定向套件通过。原生频道、SMS/passkey provider、支付/邮件/AI 等外部依赖仍不计入生产验收。
- 本轮只重启了隔离项目的 `teamgram`、`gateway`、Kafka 和 Jaeger 容器；生产 `backend-backend-1`、生产数据库及其它服务未重启、未写入。日志中仅保留已知的 `auth.initPasskeyLogin -> 400 METHOD_NOT_IMPL` fail-closed 请求，没有业务 500 或超时。

## 本轮第三十三次复核：统计权限与最终虚拟用户回归

- `stats.getBroadcastStats` 和 `stats.getMegagroupStats` 现在校验频道存在、`access_hash`、成员身份和管理员权限；错误 hash 返回 `CHANNEL_INVALID`，普通成员返回 `CHAT_ADMIN_REQUIRED`，创建者可读回类型化统计。`TestStatsRejectsInvalidHashAndNonAdmin` 在隔离审计库通过。
- `account.resetNotifySettings` 不再把 UserClient 错误伪装成 `BoolFalse` 成功响应；错误原样传播并返回 nil，`TestAccountResetNotifySettingsPropagatesUserError` 通过。
- 14 用户真实 WebSocket 回归最终通过：`business-flow acceptance passed: isolated-e2e-all-final-1790480644`。流程包含联系人、屏蔽、私聊最终一致性读回、群组、频道、阅读数、差分、共同会话、统计 owner/普通成员权限边界和清理。
- 真实浏览器回归在为群成员搜索增加 750ms 节流窗口后通过：`npm run test:playwright:real -- --reporter=line`，1/1，46.6 秒。未留下 Playwright/Vite/GramJS 测试进程；隔离 MySQL、Kafka、Redis、etcd、Jaeger、MinIO、teamgram 和 gateway 均保持健康。生产容器和生产数据库未操作。

## 本轮第三十四次复核：真实手机号联系人导入

- Web 客户端的精简 GramJS TL 白名单漏掉了 Layer 229 已定义的 `contacts.getSaved` 和 `contacts.deleteByPhones`。两项已加入 `telegram-tt-master/src/lib/gramjs/tl/static/api.json` 并通过 `npm run gramjs:tl` 重新生成；运行时 `Api.contacts.GetSaved` 和 `Api.contacts.DeleteByPhones` 均为可构造的函数。此问题发生在客户端请求编码之前，不能作为后端接口失败计数。
- 隔离 14 用户 GramJS 回归已用新构造器完整通过：`business-flow acceptance passed: isolated-contact-fix-1790497757`。Alice 用 E.164 格式 `+12025550103` 导入已注册 Carol，`contacts.importContacts` 返回对应 `ImportedContacts` 用户和 client ID，`contacts.getSaved` 回读规范化数字串 `12025550103`，`contacts.getContacts` 回读 Carol；清理阶段再用同一个 E.164 号码调用 `contacts.deleteByPhones`，确认返回 true 且 `contacts.getSaved` 不再保留该号码。
- 同一运行继续完成现有私聊、群组、频道、搜索、阅读、草稿、置顶、转发、阅读数、统计权限和清理流程。隔离项目的 teamgram、gateway、MySQL、Redis、etcd、Kafka、MinIO 和 Jaeger 保持运行；日志中的 `CHAT_ADMIN_REQUIRED` 来自脚本刻意验证的普通成员拒绝。没有发现 panic、`METHOD_NOT_IMPL`、`PeerStories.Encode` 崩溃或业务超时。一次早期连接关闭产生的 `context canceled`/连接清理日志没有导致回归失败，完整重跑已通过。
- 这是一组隔离环境的真实 session/BFF/业务服务验收证据，只覆盖该业务流涉及的方法；它不将 813 个 Layer 229 方法宣布为生产级全部通过。外部短信、支付、邮件、AI provider、破坏性账户操作和台账中其它静态缺口仍需单独验收或实现。

## 本轮第三十五次复核：联系人发现与无状态联系人

- `contacts.getContactIDs`、`contacts.getStatuses` 和用户类型的 `contacts.resolveUsername` 已加入 GramJS 精简 Layer 229 白名单；重新生成后运行时构造器均可用。真实 14 用户流程验证联系人 ID 含 Bob 和 Carol，用户名解析返回 Carol 的用户实体。
- 有效联系人列表中，Carol 没有 `user_presences` 行。原 `contacts.getStatuses` 只为数据库返回的 last-seen 行输出状态，导致响应向量漏联系人；现在按联系人列表逐个返回 `ContactStatus`，缺少 last-seen 时使用合法的 `userStatusEmpty`。新增 focused 测试覆盖一个联系人有 last-seen、另一个没有的情况，`go test ./app/bff/contacts/internal/core -run 'TestContactsGetStatuses' -count=1` 通过。
- 完整隔离 14 用户回归通过：`business-flow acceptance passed: isolated-contacts-expanded-1790499791`。本次还验证了 `contacts.getContactIDs`、`contacts.getStatuses`、`contacts.resolveUsername`，以及上一轮的 E.164 导入、saved 联系人读回与清理；原有私聊、群组、频道及权限流程也通过。日志未见 panic、`METHOD_NOT_IMPL`、内部 500 或业务超时。
- 本轮新增测试验证 `contacts.deleteByPhones` 的未注册号码删除按调用者隔离，并扩展频道用户名更新/解析；证据和限制见第三十六次复核。该隔离会话验收不提升完整生产链路计数，完整生产端到端仍为 **0/813**。

## 本轮第三十六次复核：未注册联系人归属与频道用户名

- 修复 `channels.updateUsername` 对 APIFull 频道创建者误报 `CHAT_ADMIN_REQUIRED`：用户名服务原先只查普通频道插件，而隔离 BFF 将该插件留空；现在同时查询已持久化的 APIFull 频道投影，并使用其中的 `creator`/管理员权限。新增测试覆盖 APIFull 创建者放行和非创建者拒绝，`go test ./app/bff/usernames/internal/core` 通过。
- `contacts.resolveUsername` 已通过 `channelview.ChatsByID` 将 APIFull 频道解析结果填入 `ResolvedPeer.Chats`。真实流程中 Bob 不属于频道，仍成功解析 Alice 刚更新的频道用户名并收到对应 `Channel` 实体；普通用户无法执行频道管理写操作的权限断言也通过。原生频道若不在 APIFull 投影且没有插件对象时仍失败关闭。
- `contacts.deleteByPhones` 对未注册号码现在按 RPC metadata 中的调用者删除导入记录。真实流程让 Alice 与 Bob 导入同一号码，分别读到 importer 数 1 和 2；Alice 删除后她自己的 saved 记录消失，Bob 的记录保留且再次导入显示 importer 数 1；Bob 删除后自己的记录也消失。号码每次运行随机生成，避免隔离数据库跨轮次残留污染计数断言。
- 完整 14 用户隔离 WebSocket 流程通过：`business-flow acceptance passed: isolated-e2e-1790503791659-1662`。涵盖联系人导入/读取/删除、屏蔽、私聊、基础群、频道消息读回与搜索、消息阅读数、频道差分、频道详情/成员、统计及权限边界。所有测试会话和创建的数据均由脚本清理。
- 构建峰值曾令隔离 MySQL 和 Kafka 退出 137；原容器及卷恢复后健康，应用在依赖恢复后重新启动，流程通过。仅操作 `teamgram-web-isolated-*` 隔离项目，没有操作生产服务或数据库。隔离流程不等于 813 个方法全部通过，完整生产端到端验收仍为 **0/813**。

## 本轮第三十七次复核：虚拟用户回归与无 provider 方法

- 以 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` 单独编译当前 `bff`，仅热替换 `teamgram-web-isolated-*` 容器中的 BFF 进程；容器内原二进制已按 SHA 保留备份。没有重建镜像，也没有重启隔离 Kafka、MySQL 或任何生产容器。
- 新 BFF 上的 `docker/web-isolated/verify-business-flows.sh` 让 14 个独立 fixture 用户完整登录并通过真实 WebSocket -> session -> BFF 路径完成联系人、E.164 导入与删除、屏蔽、私聊收发/搜索/已读/编辑/置顶/转发/草稿、14 人群组、频道消息/历史/阅读数/搜索/差分/详情/成员及统计权限边界。结果：`business-flow acceptance passed: isolated-e2e-1790519391112-36405`。运行未见业务 `METHOD_NOT_IMPL`、500 或超时。
- 隔离 Web Playwright 套件在该 BFF 上 3/3 通过：联系人新增/删除、群组创建与消息回读、私聊消息发送与编辑。浏览器覆盖使用独立账号和真实 WebSocket，不把 UI 断言计为其它未调用 API 的验收。
- `stats.getBroadcastStats` 和 `stats.getMegagroupStats` 不再从无写入方的调用者 boost KV 键取成员数，改为读取 APIFull 持久化频道成员表。`TestStatsUsePersistedChannelMemberCount` 在旧 KV 同时存在时验证两个统计仍返回真实 2 人成员数；其它分析字段仍缺权威数据。`stats.getBroadcastRevenueStats` 和 `stats.getBroadcastRevenueTransactions` 在缺少收入 provider 时改为 `METHOD_NOT_IMPL`，不再返回空成功对象。
- `channels.getChannels` 现使用已保存频道并验证 access hash，`channels.getFullChannel` 的成员/管理员计数来自持久化 roster；`TestChannelsGetChannelsUsesStoredChannelAndValidatesHash` 通过。没有数据后端的 `channels.editPhoto`、`getAdminLog`、`setStickers`、`getGroupsForDiscussion`、`setDiscussionGroup`、`getInactiveChannels` 均明确返回 `METHOD_NOT_IMPL`；相应 focused test 通过。
- Passkey 注册、读取、删除和完成登录因没有可信 RP、attestation/assertion verifier 与公钥凭据存储而失败关闭；群通话流媒体通道、RTMP URL 和 Stars 捐赠读取因没有媒体控制面与账本而失败关闭。`go test ./app/bff/passkey/internal/core -count=1`、`go test ./app/bff/qrcode/internal/core -count=1` 以及群通话 focused test 通过。
- Stars、礼物、boost、付费媒体/消息/reaction、订阅、推广、giveaway 与礼物集合等没有 provider 的 handler 统一在认证后返回 `METHOD_NOT_IMPL`，不再读取或写入本地模拟支付状态。`TestPaidProductMethodsFailClosedWithoutPersistence` 覆盖银行识别、付费媒体/消息/reaction 与收入读取，确认失败路径没有持久化副作用；Premium store purchase focused test 也通过。支付 provider、账本、库存和结算仍未实现。
- 本轮 `go test ./app/bff/apifull/internal/core -count=1` 通过，并通过 `go test -c` 编译 APIFull 与 Premium core，`git diff --check` 通过。隔离 MySQL focused tests 仅连接 `layer229-audit-mysql/teamgram_audit`，临时账号在每次测试后删除；默认 `teamgram` 数据库、生产服务和生产容器没有被访问或修改。
- 重新比较 `../telegram-api/methods.json` 与台账：813 个唯一方法、813 行唯一台账、名称和构造器 ID 无缺失、无多余、无不匹配。当前审计状态为 457 项已知静态缺口、265 项仅路由、28 项部分隔离库、13 项隔离库、18 项部分组件、8 项隔离组件、11 项部分 session、2 项 session 分发待验收和 11 项传输包装。生产状态为 448 项实现缺口阻塞、266 项未验收、30 项部分隔离库、13 项隔离库、20 项部分组件、8 项隔离组件、14 项部分隔离 session、3 项隔离 core 和 11 项传输包装；完整生产端到端验收仍为 **0/813**。

## 本轮第三十八次复核：定时消息、公开频道链接与浏览器虚拟用户

- 三条并行审计分别完成定时消息生命周期、公开频道消息链接和真实 Web UI 工作流。Scheduled RPC 现在按调用者与 peer 隔离待发记录；未来时间的 `messages.sendMessage` 写入队列，`getScheduledHistory`/`getScheduledMessages` 可读回，`deleteScheduledMessages` 删除记录，`sendScheduledMessages` 经现有 `MsgSendMessageV2` 投递后才消费记录。过去时间和 `InputPeerEmpty` 分别返回 `SCHEDULE_DATE_INVALID` 与 `PEER_ID_INVALID`。
- `channels.updateUsername` 成功后同步 `apifull_channel.username`；`channels.exportMessageLink` 校验精确 access hash、调用者成员身份、公开用户名和消息存在性，并返回 `https://t.me/<username>/<id>`。成员与创建者均可导出；非成员、错误 hash、私有频道分别返回 `USER_NOT_PARTICIPANT`、`CHANNEL_INVALID`、`CHANNEL_PRIVATE`。Grouped/thread 链接仍明确 `METHOD_NOT_IMPL`。
- Web 的精简 GramJS schema 漏掉了 `messages.getScheduledMessages`。已将其加入 `telegram-tt-master/src/lib/gramjs/tl/static/api.json` 并运行 `npm run gramjs:tl`；这是调用前的客户端构造器缺口，不计为后端接口失败。
- 通过的定向验证：`go test -run '^TestStoredScheduledDeliveryValidatesDateAndPeer$' -count=1 ./app/bff/messages/internal/core`；使用仅限 `127.0.0.1:13306/teamgram_audit` 的 `TestScheduledMessagesLifecycleScopesPeerAndConsumesSentRecords`、`TestChannelsExportMessageLink...` 和 `TestChannelsUpdateUsernamePersistsAPIFullChannel`。凭据没有输出或写入文件。
- 以 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` 统一构建 BFF，SHA-256 为 `16058d81abf1bb641b40442d1c7fe00a88789d17a1a360d0e6135beea42e5cba`。只热替换 `teamgram-web-isolated-*` 内的 BFF；旧二进制保存为 `bff.pre-scheduled-links-93a7c1ed27544e3d`。MySQL、Kafka、Redis、网关、消息服务和任何生产容器均未重启或修改。
- 新的双用户 WebSocket probe 完整通过：定时排队、按 ID 查询、取消、立即投递、接收方普通历史读回以及两个负向输入；频道链接 probe 通过创建者/成员链接和三类拒绝。随后 14 用户主流程 `isolated-e2e-1790529638979-76821`、媒体、资料/隐私/草稿、频道成员、频道消息、反应、群通话控制均通过。一次频道成员 probe 在认证前停滞且未发送任何 `channels.*` RPC；终止该孤立进程后以受控运行完整通过，不计为业务失败。
- `npm run test:playwright:real` 在新 BFF 和新 GramJS schema 上通过 4/4：运行时构造器、联系人、新建群并收发消息、私聊发送/编辑/收件人读回。测试定位器不再依赖会被资料流程修改的显示名。
- 重解析 `../telegram-api/methods.json` 与台账后，813 个名称和 constructor ID 仍完全匹配。当前审计状态为 446 项静态缺口、253 项仅路由、25 项部分隔离库、13 项隔离库、18 项部分组件、8 项隔离组件、37 项部分隔离会话、2 项 session 分发待验收和 11 项传输包装；生产接受状态为 438 项实现缺口阻塞、254 项未验收、26 项部分隔离库、13 项隔离库、20 项部分组件、8 项隔离组件、40 项部分隔离 session、3 项隔离 core 和 11 项传输包装。
- 这些是隔离环境的真实 session/BFF/业务服务证据，不将 813 个 Layer 229 方法宣布为生产级。SMS、邮件、支付、跨 DC 授权、广告、媒体实时通道及其余缺少权威 provider/数据模型的方法仍保留 fail-closed 或实现缺口状态。

## 本轮第三十九次复核：授权转移与隔离 WebSocket 证据

- `auth.exportAuthorization` 当前校验来源永久 auth key，并在 Redis 写入 32 字节随机凭据，TTL 为五分钟；`auth.importAuthorization` 会先读取并验证来源 key 归属及不可变用户，再原子消费凭据、绑定目标 key 并返回真实用户。新增 DAO/core 用例覆盖失效来源和缺失用户不会消耗凭据，以及两个并发导入只有一个可成功。`dc_id` 仍只做输入校验；因 Redis 消费和 authsession 绑定跨服务，绑定 RPC 失败仍会消耗凭据。
- 隔离 WebSocket 基线在 pre-candidate BFF 上创建真实私聊，完成 dialog filter 保存/排序/标签开关、Alice/Bob 隔离、`folders.editPeerFolders` 的自定义文件夹/归档/移出和跨用户 `FOLDER_ID_INVALID`，并通过 `messages.markDialogUnread`/`messages.getDialogUnreadMarks` 回读未读 peer。`getDialogUnreadMarks` 的 parent-peer 范围未覆盖，`messages.getSuggestedDialogFilters` 仍为 `METHOD_NOT_IMPL`；这些结果不证明 candidate 的运行时行为。
- 同一 pre-candidate 基线创建基础群，导出、检查并导入普通邀请，导出/检查需申请邀请并收到预期 `INVITE_REQUEST_SENT`，读取 owner 的导出列表和待处理导入者列表；成员导出/列表拒绝也在后续失败前通过。旧 BFF 的 `messages.hideChatJoinRequest` 对成员返回 `PEER_ID_INVALID`，未返回预期 `CHAT_ADMIN_REQUIRED`，因此没有提升 `messages.hideChatJoinRequest`、`messages.hideAllChatJoinRequests`、`channels.toggleJoinRequest` 或 `channels.toggleJoinToSend`；频道域持久化仍缺失。
- 本轮台账计数为 443 项静态缺口、244 项仅路由、23 项部分隔离库、13 项隔离库、18 项部分组件、8 项隔离组件、51 项部分隔离 session、2 项 session 手动分发待验收和 11 项传输包装；生产列为 435 项实现缺口阻塞、245 项未验收、24 项部分隔离库、13 项隔离库、20 项部分组件、8 项隔离组件、3 项隔离 core、54 项部分隔离 session 和 11 项传输包装。没有部署 candidate，完整生产级端到端验收仍为 **0/813**。

## 本轮第四十次复核：授权转移预检和最终虚拟用户回归

- 仅热替换隔离 `teamgram-web-isolated-*` 容器中的 `bff`，新二进制 SHA-256 为 `e28c0e4403ee3db2c1271d7b71f1e3859cb574f41456af97ddd6f039d5e4b755`，旧二进制保留为 `/app/bin/bff.pre-auth-prevalidate-20260928`。没有重建镜像、重启 session、数据库或其它服务。
- `probe-upload-repro.ts` 使用持久 auth key 完成主连接上传、同 DC worker 上传，以及经 `auth.exportAuthorization` / `auth.importAuthorization` 后 `targetDc=1` worker 上传。它证明隔离环境中的 session → BFF → Redis → authsession 传递链可用；RPC metadata 没有独立目的 DC 身份，因此不宣称生产级 DC 隔离。凭据过期、重启持久性和绑定 RPC 失败后的可重试性仍未验收。
- 当前 BFF 上，`verify-business-flows.sh` 让 14 个 fixture 用户重新完成联系人、私聊编辑后的收件人读回、群组、频道、阅读数、差分、群通话控制和统计权限流程：`business-flow acceptance passed: isolated-e2e-1790537480117-12373`。
- 真实浏览器套件 `npm run test:playwright:real` 最终 4/4 通过。一次直接消息 UI 失败被定位为测试在全局搜索结果中无条件点击第一个条目，实际选择了 `isolated_test_03`；测试现改为按目标用户名匹配唯一结果，再验证收件人读取编辑后的消息。该修正不改变服务端消息逻辑。
- 当前台账审计状态为 443 项静态缺口、244 项仅路由、23 项部分隔离库、13 项隔离库、16 项部分组件、8 项隔离组件、53 项部分隔离 session、2 项 session 手动分发待验收和 11 项传输包装；生产列为 435 项实现缺口阻塞、245 项未验收、24 项部分隔离库、13 项隔离库、18 项部分组件、8 项隔离组件、3 项隔离 core、56 项部分隔离 session 和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第四十一次复核：频道邀请、网页预览与读取防御

- `messages.readHistory` 现在在 BFF 层验证认证上下文、请求、peer、`max_id` 和 MsgClient 依赖；无效 peer、负值、后端错误和 nil 成功回复均失败关闭。focused 测试覆盖 self/chat 映射、六类无效输入不发 RPC、后端错误和 nil 回复。频道 peer 仍不支持，且没有完整 session/BFF/数据库读写验收。
- 频道邀请完整隔离 session 流程已覆盖频道创建、邀请导出/读取/编辑/撤销/删除、普通和申请加入、单个与批量审批、导入者和管理员邀请列表、权限拒绝、`channels.toggleJoinRequest`、`channels.toggleJoinToSend` 及错误 access hash。两项 toggle 返回的 Chat 已补齐必填 `Photo`，避免 session 编码空指针。该实现依赖 `migrate-20260929-chat-invite-request-columns.sql`；生产启用前必须执行迁移。
- `messages.getWebPagePreview` 和 `messages.getWebPage` 不再保存调用者文本或回显旧 note。它们只接受安全 HTTP(S) URL，规范化 URL、生成稳定 ID/hash 并持久化 URL 派生记录；哈希匹配返回 `webPageNotModified`，私网/loopback/凭据 URL 被拒绝。真实隔离 GramJS 登录已验证 preview、完整读取、not-modified、私网拒绝和 localhost 空预览。没有远程抓取或 OpenGraph provider，因此标题、描述、图片和页面内容仍不可用。
- `account.getGlobalPrivacySettings` / `account.setGlobalPrivacySettings` 已完成 BFF 到 user core/DAO 的错误传播和 Layer 229 字段编码，包括 gifts 按钮、非联系人 Stars 价格与 gift 限制 JSON。隔离栈先执行 `migrate-20260929-global-privacy-settings.sql`，再由 `probe-global-privacy-settings.ts` 通过真实 WebSocket -> session -> BFF -> biz user -> MySQL 为 fixture user `136907713` 写入并读回全部支持字段；直接查询隔离 MySQL 也确认了持久化行。验证随后恢复默认值，并显式删除这次新建的 fixture 行。focused BFF/user-core 测试同样通过；没有生产流量或写入后重启持久性验收，生产部署仍需执行该迁移并重启 privacysettings BFF 和 biz user 服务。
- 中断的频道邀请 probe 遗留数据仅在隔离 MySQL 中清理：已删除已知孤立频道对应记录，复查 `all_orphan_invites=0`、`all_orphan_participants=0`。probe 的 finally 现在追踪导出链接、删除仍有效的 owner 邀请，再删除测试频道。生产数据库未操作。
- 当前 ledger 以 CSV parser 重算：432 项已知静态缺口、205 项仅路由、99 项部分隔离 session、25 项部分组件、21 项部分隔离库、10 项隔离库、8 项隔离组件、2 项 session 手动分发待验收和 11 项传输包装。生产状态为 427 项实现缺口阻塞、206 项未验收、102 项部分隔离 session、27 项部分组件、22 项部分隔离库、10 项隔离库、8 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第四十二次复核：授权 provider 失败关闭

- `auth.importBotAuthorization` 仍可查询真实 bot token，但当前授权 BFF 没有可信的 `api_id/api_hash` 注册表；合法形状的 API 凭证不再触发 authsession 绑定，认证后明确返回 `METHOD_NOT_IMPL`。`auth.importWebTokenAuthorization` 同样要求非空 token、认证上下文和 API 形状后，在没有 web-token verifier 时返回 `METHOD_NOT_IMPL`。
- `auth.dropTempAuthKeys` 现在区分无效永久 auth key、空请求和缺失临时密钥后端：没有临时密钥存储或 authsession drop RPC 时返回 `METHOD_NOT_IMPL`，不再把永久 key 报成 `AUTH_KEY_INVALID` 或返回 `BoolTrue`。
- `auth.requestFirebaseSms` 在验证 Firebase token 的手机号与已签发 phone-code 后仍需 SMS provider；没有 provider 时返回 `METHOD_NOT_IMPL`，不会把 token attestation 当成短信发送成功。`auth.reportMissingCode` 没有 provider 事故/报告 sink 时返回 `METHOD_NOT_IMPL`，不把本地 phone-code 记录当作已上报。`auth.checkPaidAuth` 没有支付表单/provider 时返回 `METHOD_NOT_IMPL`，不再合成 `auth.sentCodePaymentRequired`。
- `auth.resetLoginEmail` 只接受未过期且处于可重发状态的已有 phone-code，并通过 shared challenge service 的真实 SMS provider 重新投递；provider 缺失时返回 `SMS_CODE_CREATE_FAILED`，不会改变成功状态。新增 authorization provider-gap focused tests；`go test ./app/bff/authorization/internal/core` 与 `go test ./pkg/code/...` 通过。APIFull `account.getTmpPassword` 和密码邮件拒绝测试仍依赖其隔离 MySQL test harness，本轮环境未设置 `APIFULL_MYSQL_DSN`，因此未运行该包。

## 本轮第四十三次复核：统计、语言包与社区 provider 缺口

- `langpack.getLangPack`、`getStrings`、`getDifference`、`getLanguages`、`getLanguage` 现在先验证认证，再返回 `METHOD_NOT_IMPL`。APIFull 没有权威翻译目录、语言元数据或版本化差分 provider；请求的 key/code、版本和本地 `messages.translateText` KV 不再被当作语言包成功响应。新增匿名/认证拒绝及本地翻译状态不外泄的 focused tests。
- `communities.create`、`togglePeerLink`、`getJoinedCommunities`、`toggleCommunityCollapsedInDialogs`、`getPeerLinkRequests`、`togglePeerLinkRequestApproval`、`toggleAllPeerLinkRequestApproval`、`toggleParticipantBanned` 现在先验证认证，再返回 `METHOD_NOT_IMPL`，不再写通用 bot KV、返回空 Updates/Bool 或合成 Community Chat。`getParticipantJoinedChats` 保留认证、社区频道和参与者 peer 校验后失败关闭；新增 provider-gap/input focused tests。
- `stats.getStoryStats` 不再从无法由当前 Stories provider 写入的本地 Story 文档合成 views graph，认证后返回 `METHOD_NOT_IMPL`。`stats.getBroadcastStats`、`getMegagroupStats` 现在要求真实 `InputChannel`，继续读取已持久化频道成员/阅读游标；缺失 channel 不生成全局零值统计。`stats.getPollStats` 要求 peer/message，校验真实 PollMessageReader 的消息与权限后读取已持久化 ballots；缺少消息 provider、错误 peer 或未知消息均失败关闭。
- 本轮仅完成 APIFull core 编译与静态审计；runtime `go test` 被仓库 `TestMain` 阻止，因为当前环境未设置 `APIFULL_MYSQL_DSN=127.0.0.1:13306/teamgram_audit`。没有访问默认/生产数据库或服务。

## 本轮第四十四次复核：Bot、Inline Bot 与 Mini App 失败关闭

- 复核了 `inline_bot.go`、`mini_bot_apps.go`、`main_mini_bot_apps.go`、`bots.go`、`bot_admin_right.go` 和 `bot_verification_icons.go`。仓库没有可用的 inline bot 执行/结果投递、callback 查询关联、Bot 注册表/用户名索引、Bot token 签发、Bot 权限/管理员关系、验证图标/推荐目录或 WebView/Mini App 授权会话 provider；BFF handler 也没有可转发的权威业务服务。
- Inline 查询/设置/发送/编辑、callback、requested peer 和 guest-chat result 现在先验证认证，再返回 `METHOD_NOT_IMPL` 和 `nil`，不再读写 caller-local cache、callback note 或 synthetic inline message ID。
- Mini App/WebView 的 request/prolong/simple/app/chat-join/main、send-result/send-data、Bot App、can/allow-send、custom method、download check 和 WebView button 方法同样先验证认证，再返回 `METHOD_NOT_IMPL` 和 `nil`；不再回显 URL/short name、生成空 WebView 结果、记录请求 JSON 或返回固定成功/空对象。
- Bot profile、admined-bot、username、create/export-token、access settings、join-chat results、默认管理员权限、custom verification、recommendations 和 popular app bots 的无 provider 路径均失败关闭。常规 Bot commands/menu、preview media 和 prepared inline messages 仍是已有的本地闭环，未被本轮的 provider-gap 结论覆盖。
- 新增 `bot_webview_unavailable_test.go` 使用 store probe 断言认证后的代表性方法返回 `METHOD_NOT_IMPL`、结果为 `nil` 且持久化读写计数为零；匿名调用优先返回 `AUTH_KEY_UNREGISTERED`。相关 inline、mini app、bots 和 admin 测试断言已同步更新。
- 已运行 `gofmt`、`git diff --check`，并成功编译 `go test -c ./app/bff/apifull/internal/core -o /tmp/apifull-core-bot.test`。完整 runtime 测试被现有 `TestMain` 阻止：当前环境缺少 `APIFULL_MYSQL_DSN=127.0.0.1:13306/teamgram_audit`；没有访问默认或生产数据库，也没有把编译结果当作业务验收。
- 重新用 CSV parser 校验台账：813 行、方法名唯一且字段列宽有效。当前静态状态为 426 项已知不完整、156 项仅路由、20 项部分隔离库、10 项隔离库、37 项部分组件、10 项隔离组件、141 项部分隔离 session、2 项 session 分发待验收和 11 项传输包装；生产状态为 422 项实现缺口阻塞、157 项未验收、21 项部分隔离库、10 项隔离库、38 项部分组件、10 项隔离组件、144 项部分隔离 session 和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第四十五次复核：频道头像、支付保存信息与通话 artifact

- `channels.editPhoto` 不再直接返回 `METHOD_NOT_IMPL`。APIFull core 对频道 owner 做授权校验，支持清空头像、复用 `InputPhoto` 和通过现有 DFS 上传后写入 `photo_id`、`photo_dc_id`、`photo_has_video`；频道 Chat 读取同一 canonical 行。`apifull_channel` 的启动迁移和 `migrate-20260922-apifull.sql` 已包含这些字段。当前只完成 core/domain/channelview 编译，隔离 MySQL 与真实 session/media 上传仍未执行。
- `payments.getSavedInfo` 和 `payments.clearSavedInfo` 现在读写认证用户的持久化支付信息及凭据标记，分别返回 `payments.savedInfo` 和清理确认；清理信息与凭据可独立执行。`TestPaymentInfoMethodsUsePersistence` 通过持久化 spy 验证读回和清理。支付 provider、支付资料的数据库模型、凭据加密存储、扣款/退款/Stars 结算仍未接入，因此不提升为完整支付实现。
- `phone.receivedCall` 现在校验 call access hash 与参与者并把 call 持久化为 `received`。`phone.setCallRating`、`phone.saveCallDebug`、`phone.sendSignalingData`、`phone.saveCallLog` 写入 `apifull_call_artifact`；评分按用户/通话覆盖重试，其余 artifact 追加，信令同时经 Sync 推送给另一参与者。组件编译和现有状态机覆盖通过；真实双用户 session、TURN/媒体协商、artifact 消费者和生产通话服务仍未验收。
- 运行 `gofmt`、`go test -c ./app/bff/apifull/internal/core`、`go test -c ./app/bff/apifull/internal/domain`、`go test -c ./app/bff/apifull/channelview` 及 `git diff --check` 均通过。APIFull runtime 测试仍要求明确的隔离 `APIFULL_MYSQL_DSN=127.0.0.1:13306/teamgram_audit`，本轮未连接默认或生产数据库，也未重启任何生产服务。
- 本轮台账只把上述方法从“静态缺口”调整为“组件级部分实现”；短信/邮件 provider、Passkey RP/attestation verifier、真实第二 DC、支付 provider、原生频道消息服务、秘密聊天跨设备密钥交换、礼物目录/库存和通话媒体控制面仍是明确缺口。完整生产级端到端验收仍为 **0/813**。

## 本轮第四十六次复核：群通话控制面持久化和权限收紧

- `phone.toggleGroupCallSettings`、`phone.editGroupCallParticipant`、`phone.toggleGroupCallStartSubscription`、`phone.startScheduledGroupCall`、`phone.joinGroupCallPresentation` 和 `phone.leaveGroupCallPresentation` 不再只保存请求。它们现在读取并更新群通话设置/参与者状态，要求正确的 call access hash 和成员/创建者权限，拒绝空更新、负数付费消息额度、未安排的定时通话和无效 presentation JSON；离开屏幕共享会清除参数和暂停状态。
- `phone.saveDefaultGroupCallJoinAs` / `phone.getGroupCallJoinAs` 现在只接受能在 canonical channel/user 数据中验证的 peer，校验频道 access hash、成员身份和用户 access hash，并按用户和目标 peer 隔离读回。`phone.saveDefaultSendAs` 同样绑定到已授权通话和真实参与者/频道。
- `phone.sendGroupCallMessage`、`phone.deleteGroupCallMessages` 和 `phone.deleteGroupCallParticipantMessages` 已接入持久化消息表，执行随机 ID、消息开关、send-as 和作者/创建者权限校验，并返回带消息/删除 ID 的 typed Updates。跨客户端广播、端到端加密链块和会议控制面仍未接入。
- `phone.toggleGroupCallRecord` 现在持久化录制控制状态，但没有录制消费者，因此不能证明生成媒体；`phone.exportGroupCallInvite` 已生成随机 bearer token、只保存摘要，并由 `joinGroupCall` 校验和消费，通知/深链投递仍未接入。`phone.getGroupCallStreamChannels`、`phone.getGroupCallStreamRtmpUrl`、`phone.getGroupCallStars` 以及 conference-call/media 方法同样保持失败关闭，避免伪造媒体、结算或加密成功。
- 本轮对 `group_calls.go` / `group_calls_test.go` 执行 `gofmt`，`go test -c ./app/bff/apifull/internal/core -o /tmp/apifull-core-final.test` 通过；运行时测试未执行，因为当前没有有效的隔离 `APIFULL_MYSQL_DSN`。CSV 重新解析通过：813 行、方法名唯一、8 列。当前台账为 388 项已知静态缺口、153 项仅路由、75 项组件部分实现、20 项部分隔离库、10 项隔离库、144 项部分隔离 session、10 项隔离组件、2 项 session 手动分发待验收和 11 项传输包装；生产级端到端验收仍为 **0/813**。

## 本轮第四十七次复核：壁纸/主题上传与对话设置写入

- `account.uploadWallPaper` 现在校验输入并调用已有 DFS `uploadWallPaperFile`，把真实的 creator、文件和 MIME 类型传入，返回 DFS 生成的带文档的 `WallPaper`；`account.uploadTheme` 同样接入 DFS `uploadThemeFile`，支持缩略图和文件名。没有 DFS 时仍返回 `METHOD_NOT_IMPL`，不会生成空文档。
- `messages.setChatWallPaper` 要求已认证 peer 和本地可验证的壁纸引用，调用 Dialog `setChatWallpaper` 持久化壁纸 ID/覆盖标记；`messages.setChatTheme` 校验 peer 和 emoticon 后调用 Dialog `setChatTheme`。两者返回类型正确的空 Updates，跨会话更新广播、远程目录和完整设置字段仍未接入。
- 上传壁纸的真实文档会按用户保存，后续 `account.getWallPaper` 和 `account.getMultiWallPapers` 可以读回该 DFS 文档；这只覆盖调用者自己的上传目录，不提供全局目录。
- 相关 core 包编译和 `git diff --check` 通过；本轮未连接隔离数据库或运行 session 业务流。CSV 重新解析为 813 行、方法名唯一、8 列：381 项已知静态缺口、153 项仅路由、82 项组件部分实现、20 项部分隔离库、10 项隔离库、144 项部分隔离 session、10 项隔离组件、2 项 session 手动分发待验收和 11 项传输包装；完整生产级端到端验收仍为 **0/813**。

## 本轮第四十八次复核：主题元数据创建与更新

- `account.createTheme` 现在只接受当前用户通过 `account.uploadTheme` 返回并在 APIFull KV 中保存的真实 `Document`，使用该文档的 ID/access hash 建立主题，保存 slug、title 和可由本地壁纸引用解析的 settings；未知或其他来源的 `InputDocument` 返回 `DOCUMENT_INVALID`。
- `account.updateTheme` 校验已保存主题和 access hash，更新请求提供的 slug/title/document/settings，并保留未修改的真实文档和元数据。没有全局主题目录、跨会话主题广播或 unique-gift/chat-theme provider，因此相关方法仍显式返回 `METHOD_NOT_IMPL`。
- 新增聚焦测试覆盖 owned-document create、真实文档持久化、title update 和 unowned-document rejection；APIFull core 编译通过，运行时测试仍受隔离 `APIFULL_MYSQL_DSN` 缺失限制。

## 本轮第四十九次复核：台账一致性、主题所有权与导入失败关闭

- `LAYER229_METHOD_LEDGER.csv` 已用结构化 CSV 重写校验：813 行均为 8 列，`method + constructor_id` 与 `../telegram-api/methods.json` 的 813 项完全一致；移除了不属于 Layer 229 schema 的旧版 `messages.getMessageReadParticipants#2c6f97b7` 记录，并修正了 11 行未转义逗号。
- 主题和壁纸的本地路径现在只接受当前用户拥有且带真实 DFS `Document`、匹配 access hash 的记录。`account.saveTheme`、`account.installTheme`、`account.getTheme`、`account.getThemes`、`account.getWallPapers` 和多壁纸读取不再接受只有 ID/slug 的伪造对象；本地 hash 命中时返回 `themesNotModified`/`wallPapersNotModified`。全局目录、远程资源和跨会话广播仍没有 provider，因此这些方法只标为组件级部分实现。
- 导入流程没有权威解析器、任务队列或消息写入 provider。`messages.checkHistoryImport`、`messages.initHistoryImport`、`messages.startHistoryImport`、`messages.checkHistoryImportPeer` 认证并校验输入后返回 `METHOD_NOT_IMPL`，不再写入本地假任务或返回 `BoolTrue`。`messages.uploadImportedMedia` 仍只保留已有 DFS 图片/文档上传组件，并拒绝没有 `media` 的元数据伪请求。
- `messages.setTyping` 在读取元数据前增加认证检查，缺失认证返回 `AUTH_KEY_UNREGISTERED` 而不触发空指针；用户和基础群路径继续使用 User/Chat/Sync provider，频道路径现通过 APIFull 成员解析并推送 `UpdateChannelUserTyping`，隔离双用户流程已通过，生产双用户投递仍未验收。
- 本轮验证：`go test ./pkg/code/...`、`go test ./app/bff/authorization/internal/core -run 'Test(Consume|Phone|Email|SignUp|Export|Import)' -count=1`、`go test ./app/bff/files/internal/core -run 'Test(MessagesGetDocumentByHash|UploadGetFileHashes)' -count=1`、`go test ./app/bff/dialogs/internal/core ./app/bff/messages/internal/core -count=1`、APIFull 隔离库定向 `Test(HistoryImport|Theme|Wallpaper)` 均通过；`go test -c ./app/bff/apifull/internal/core` 和 `git diff --check` 通过。隔离测试数据库只在 `127.0.0.1:13306/teamgram_audit` 临时创建并已删除。
- 当前台账重新统计：357 项已知静态缺口、152 项仅路由、106 项部分组件、20 项部分隔离库、10 项隔离库、145 项部分隔离会话、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；完整生产级端到端验收仍为 **0/813**。支付结算、短信/邮件外部 provider、Passkey RP、真实第二 DC、原生频道服务、秘密聊天密钥、礼物交易目录和通话媒体控制面仍未接入。

## 本轮第五十次复核：授权导入绑定确认

- `auth.importAuthorization` 在 Redis `Ready -> Binding` claim 后，现在要求 `authsession.bindAuthKeyUser` 返回非零 bind hash，并再次读取目标 auth key 的持久化 owner；只有确认 owner 与导出用户一致时才会把 Redis 凭据标记为 `Complete` 并返回授权。
- authsession 返回 nil/zero、返回错误且无法确认 owner、或读回其它用户时，导入不会伪造成功，也不会消费凭据；Redis 保留同一目标 key 的 `Binding` claim，允许重试。新增 core focused test 覆盖 nil/zero bind 响应，DAO 原子 claim/complete 与竞争目标测试继续通过。
- 如果 Redis `Complete` 已提交但完成 RPC 的响应丢失，后续同一目标 key 的导入会校验已绑定用户并幂等重试完成，不会把已完成凭据误报成无效；其它目标 key 仍拒绝重放。
- 本轮验证：`go test ./app/bff/authorization/internal/core` 通过。该修复仍是 Redis 与 authsession 两个服务之间的可恢复编排，不是跨服务事务；真实独立第二 DC、重启/过期故障注入和生产验收仍未完成。没有远端 DC 配置时，导出/导入继续 fail-closed。

## 本轮第五十一次复核：举报持久化 intake

- `account.reportPeer`、`account.reportProfilePhoto`、`messages.reportSpam`、`messages.report`、`messages.reportEncryptedSpam`、`messages.reportReadMetrics`、`messages.reportMusicListen`、`channels.reportSpam` 和 `stories.report19D8EB45` 已接入 APIFull 的 `apifull_report` 持久化 intake。所有入口先要求认证；校验 peer/channel/encrypted-chat/photo/document/reason/message-ID/metric 结构，使用 actor、方法、目标和完整协议 payload 生成 SHA-256 去重键；数据库写入失败时不返回成功。重复请求只更新 `updated_at`，不会生成第二条逻辑记录。
- `messages.report` 只有在持久化成功后返回 `reportResultReported`；其它方法返回协议要求的 `BoolTrue`。这表示举报已进入待处理队列，不表示审核、封禁或处罚已经完成。数据库未打开时仍返回 `METHOD_NOT_IMPL`，保持无 provider 部署的 fail-closed 行为。
- 隔离验证使用 `127.0.0.1:13306/teamgram_audit`：`TestReportHandlersPersistAllSupportedMethods` 逐一调用 10 个支持入口并确认 10 条 actor-scoped intake 行，`TestReportIntakeIsDurableAndIdempotent` 验证重复提交只有一条记录、重新打开数据库后仍可按 dedupe key 读回；两项均通过。测试产生的随机 actor 行已清理，生产数据库未连接。
- 新增 `teamgramd/deploy/sql/migrate-20261001-report-intake.sql`，生产可在重启 APIFull 前显式创建同一张 `apifull_report` 表；运行时迁移仍保持幂等。
- `messages.report8953AB4E` 也复用了同一 intake helper，并覆盖在 APIFull core 测试中；该构造器不在当前 Layer 229 ledger 的 813 项业务方法集合内，因此没有新增账本行。
- 台账重新用 CSV parser 校验为 813 行、方法/构造器集合无缺失或多余；当前审计状态为 349 项已知静态缺口、152 项仅路由、106 项部分组件、28 项部分隔离库、10 项隔离库、145 项部分隔离会话、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装。生产状态为 362 项实现缺口阻塞、152 项未验收、98 项部分组件、21 项部分隔离库、10 项隔离库、148 项部分隔离会话、1 项额外的部分隔离库验收、10 项隔离组件和 11 项传输包装；完整生产级端到端验收仍为 **0/813**。
- 举报审核消费者/处罚服务、支付与 Stars 结算、礼物库存/所有权、Passkey 可信 RP、真实第二 DC、原生频道 provider、Stories provider、通话媒体控制面和私密聊天跨设备密钥仍是上线阻塞项。

## 本轮第五十二次复核：频道内容已读回执

- `channels.readMessageContents` 现在要求真实 `InputChannel` access hash、频道成员身份和非空、唯一、正的消息 ID；消息必须存在于 `apifull_channel_message`，随后在 `apifull_channel_message_content_read` 以用户/频道/消息主键幂等写入已读回执，数据库写入失败不会返回 `BoolTrue`。
- 新增 `migrate-20261001-channel-message-contents.sql`，并把频道删除、单条消息、频道历史和参与者历史删除事务纳入该表清理。域层测试覆盖重复调用只保留一条记录、整批 unknown/duplicate/invalid ID 原子失败、非成员/错误 access hash 拒绝及删除清理；APIFull core 定向测试验证类型化 `BoolTrue` 和错误传播。
- 本轮通过 APIFull core/domain compile-only、`gofmt` 和 `git diff --check`；当前环境未设置隔离 `APIFULL_MYSQL_DSN`，因此运行时数据库测试未执行。未重启生产容器或连接生产数据库。
- 该实现覆盖的是持久化 content-read receipt；频道消息仍只保存文本，完整媒体 unread provider、跨会话更新广播和 Layer 229 全链路仍未验收。台账当前为 813 行：347 项静态缺口、152 项仅路由、30 项部分隔离库、10 项隔离库、106 项部分组件、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 361 项实现缺口阻塞、152 项未验收、22 项部分隔离库、10 项隔离库、1 项已验证的部分隔离库、98 项部分组件、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十三次复核：回复消息屏蔽工作流

- `contacts.blockFromReplies` 已接入消息、用户、msg 和 sync provider：先按当前用户读取并验证入站私聊消息及作者，再持久化屏蔽；`delete_message`、`delete_history` 和 `report_spam` flags 分别调用消息删除/历史清理和持久化举报 intake，最后发送 `updatePeerBlocked`。任何缺失 provider 或 provider 错误都会失败关闭，不返回伪造 Updates。
- 本轮只完成 APIFull core compile-only 与格式检查；没有隔离数据库、真实会话、生产 `backend-backend-1` 或重启持久化验收。删除/历史清理的跨用户副作用、举报消费者和同步广播仍需端到端核对。
- CSV 结构化重读为 813 行、方法名/构造器唯一：346 项已知静态缺口、152 项仅路由、107 项部分组件、30 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 360 项实现缺口阻塞、152 项未验收、99 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十四次复核：多媒体发送请求边界

- `messages.sendMultiMedia` 现在在进入写入 provider 前校验认证、peer、非空 media 向量、每个 media 项、caption 长度和真实 `InputMedia`；结构化回复保留来源 peer，Story reply 没有权威 provider 时明确返回 `METHOD_NOT_IMPL`，缺少 Msg provider 返回服务端错误而不是空 Updates。既有 stored delivery 和 `MsgSendMessageV2` 持久化路径保持不变。
- 新增 focused handler tests 覆盖输入拒绝、缺少媒体、缺少 provider 和 Story reply；messages core 测试通过。尚未做真实多媒体发送、收件端回读、隔离库/会话或生产验收。
- 台账仍为 813 行：345 项已知静态缺口、152 项仅路由、108 项部分组件、30 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 359 项实现缺口阻塞、152 项未验收、100 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十五次复核：置顶对话频道实体同步

- `messages.reorderPinnedDialogs` 的 dialog provider 写入保持不变；更新广播现在通过 bff 配置的 channelview resolver 回填频道 `Chat` 实体，跨会话客户端不会再只收到没有频道对象的 `updatePinnedDialogs`。resolver 缺失时仍不伪造频道实体。
- 本轮通过 dialogs 与 bff server compile-only 及格式检查；没有运行隔离数据库、双会话同步或生产 `backend-backend-1` 验收。频道权限、重启后的置顶顺序和同步消费仍需真实业务流核对。
- 台账当前为 813 行：344 项已知静态缺口、152 项仅路由、109 项部分组件、30 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 358 项实现缺口阻塞、152 项未验收、101 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十六次复核：用户名更新的认证和同步失败边界

- `account.updateUsername` 现在先拒绝缺失认证、空请求、未配置 User/Sync provider，再校验用户名格式。User provider 返回空用户、空 Bool 或错误时不会继续生成响应；同步通知返回错误或空响应时会明确失败，保留“资料和用户名索引已保存、同步待重试”的可诊断状态，不再静默成功。
- 新增 focused tests 覆盖未认证、nil 请求、空 User 响应、User 写入失败、空/错误 Sync 响应和错误用户名；用户名 DAO 的隔离 MySQL 原子写入、重复占用拒绝和事务回滚测试继续作为持久化证据。
- 本轮通过 `go test ./app/bff/usernames/...`、`gofmt` 和 `git diff --check`；未连接生产数据库、未重启 `backend-backend-1`，也未宣称多会话同步已经完成。CSV 重新解析为 813 行且方法/构造器唯一：343 项已知静态缺口、152 项仅路由、109 项部分组件、31 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态仍为 358 项实现缺口阻塞、152 项未验收、101 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十七次复核：已保存对话置顶的频道实体同步

- `messages.toggleSavedDialogPin` 现在校验认证、请求和 Dialog/Sync provider 响应，传播持久化或同步错误；频道 peer 通过现有 `channelview` resolver 回填 `Chat` 到 `updateSavedDialogPinned`，用户和基础群仍使用各自权威 provider。此前频道分支的 TODO 不再静默丢失实体。
- 新增 focused tests 覆盖频道 `Chat` hydration、Dialog/Sync 错误传播以及未认证/nil 请求拒绝。测试通过 `go test ./app/bff/savedmessagedialogs/internal/core -count=1`；未连接隔离或生产数据库，未重启 `backend-backend-1`，完整 saved-dialog 会话和重启持久化仍待验收。
- CSV 重新解析为 813 行且方法/构造器唯一：342 项已知静态缺口、152 项仅路由、110 项部分组件、31 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 357 项实现缺口阻塞、152 项未验收、102 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十八次复核：搜索计数器的媒体 filter 映射

- `messages.getSearchCounters` 现在把底层消息 DAO 已支持的 `photos`、`video`、`photoVideo`、`document`、`url`、`gif`、`music`、`voice`、`roundVoice`、`roundVideo`、`chatPhotos` 和 `phoneCalls` 全部映射到规范媒体类型；同时增加认证、peer、provider 和空响应校验。此前这些合法 filter 会被误判为 `METHOD_NOT_IMPL` 的部分已恢复，poll/mentions/geo 等确实没有计数后端的 filter 仍失败关闭。
- 新增 focused tests 覆盖新增五类 filter、类型化计数、未认证和缺失 provider；BFF messages、biz-message core 和 MySQL DAO 测试通过。未连接隔离或生产数据库、未重启 `backend-backend-1`，完整搜索 session/pagination 仍待验收。
- CSV 重新解析为 813 行且方法/构造器唯一：341 项已知静态缺口、152 项仅路由、111 项部分组件、31 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 356 项实现缺口阻塞、152 项未验收、103 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第五十九次复核：PeerSettings 的频道 provider 接线

- `messages.getPeerSettings` 增加认证、nil 请求、空 provider 和空 peer 防护；用户、基础群和频道实体都要求精确 hydration/access hash，频道路径使用 Dialogs BFF 已配置的 `contactsChannelPlugin -> channelview.ChatsByID`，不再因 handler 内只检查 plugin 状态而被误记为无频道后端。
- 新增认证和 nil 请求 focused tests；原有用户/群/频道实体、成员、access hash 和 malformed-response 测试继续通过。未连接隔离或生产数据库、未重启 `backend-backend-1`，完整 session 读回仍待验收。
- CSV 重新解析为 813 行且方法/构造器唯一：340 项已知静态缺口、152 项仅路由、112 项部分组件、31 项部分隔离库、10 项隔离库、145 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 355 项实现缺口阻塞、152 项未验收、104 项部分组件、22 项部分隔离库、10 项隔离库、148 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 本轮第六十次复核：消息线程接入真实历史

- `messages.getReplies` 不再对普通用户/基础群直接返回 `METHOD_NOT_IMPL`。APIFull 先从消息服务读取并校验根消息，再分页读取同一 peer 的权威历史，按 `MessageBox` 的索引字段和协议消息里的 `reply_to` 头过滤回复，并回填用户/群实体。频道 peer 仍明确失败关闭，因为原生频道历史尚没有可重放的回复索引或关联讨论群映射。
- `messages.getDiscussionMessage` 对普通用户/基础群返回真实根消息、`messages.DiscussionMessage`、群和用户实体，并读取已有的单调 discussion read cursor；频道关联讨论仍未伪造。
- 修复了消息服务只在 `message_data.reply_to` 保留回复关系、未填充 `MessageBox.ReplyToMsgId` 的实际生产映射。新增 focused test 覆盖只存在协议 reply header 的记录。
- 重新编译为 Linux ARM64 bff（`sha256=3adf5fb3c6a0cc53c2ce45411713114bfb65a53a35b2a11345bddaa8187a5de2`），只替换 `backend-backend-1` 的 bff 进程；容器、MySQL、Redis、Kafka、etcd、DFS、msg 和其它进程没有重启，容器 `restart_count=0`。
- 真实生产 DC1 授权流程在群 `120032` 的消息 `23` 上发送了一条临时回复，`messages.getReplies` 返回 `messages.Messages` 且命中回复消息 `36`，随后删除测试消息；`messages.getDiscussionMessage` 返回 `messages.DiscussionMessage` 且包含根消息。再次调用空回复页也返回类型化结果。流程没有超时、500、panic 或 fatal。
- 这两项现标为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。仍未完成的部分是频道回复索引、链接讨论群、准确的全量分页计数、未读回复统计和重启后业务流验收；没有执行任何生产迁移。

## 本轮第六十一次复核：支付 provider 配置链与生产 bff 热替换

- APIFull 的外部支付 provider 已有 endpoint/key/timeout 读取和“provider 明确 verified 后才结算”的持久化账本；本轮补齐顶层 bff 配置结构与 `server.go` 到 APIFull 的传递链。未配置 endpoint 时继续返回 `PAYMENT_UNSUPPORTED`，不会把客户端凭据或收据当作已扣款。
- 当前生产 `backend-backend-1` 运行的是 Linux ARM64 bff，SHA-256 为 `a668256db4a91d4809c71626b16ffbc7f73fbdba561ee054e1c4316c03cc40b3`，PID 530；旧二进制保留为容器内 `/app/bin/bff.before-20261002`，主容器 `restart_count=0`。
- 仅停止并重启了 bff。MySQL、Redis、Kafka、etcd、DFS、msg、sync、session 和 gnetway 没有重启。bff 启动日志无 panic、fatal、监听冲突或启动错误；通过真实生产 WebSocket 未授权探针返回 `help.config`、`help.nearestDc`、`help.appConfig`、`help.countriesList` 类型化结果。
- 生产 MySQL 已确认 `apifull_payment_request`、`apifull_payment_ledger`、`apifull_payment_receipt`、`apifull_stars_offer` 和 `apifull_gift` 表存在；没有手工执行迁移或写入业务测试数据。隔离数据库的支付账本/礼物/Stars 和通话 focused tests 通过。
- 这轮不提升生产支付状态：仍没有真实 provider endpoint、签名验证、扣款/退款回调、Stars 充值结算或礼物目录库存数据；Passkey 仍缺可信 RP/origin/attestation 配置，第二 DC、原生频道 provider、通话 relay/media control plane 和私密聊天跨设备密钥仍阻塞。完整 813 项生产级端到端验收仍未完成。

## 本轮继续：账户注销与通话录制控制状态边界

- `account.deleteAccount` 现在拒绝 nil/未认证或缺失下游依赖，检查用户、用户名和 authsession 返回值，并传播每条 `SyncUpdatesMe` 与解绑错误；不会在用户删除或会话撤销失败时返回成功。关联消息、媒体、联系人等业务数据仍由用户服务的软删除能力决定，尚无跨服务清理事务。
- `phone.toggleGroupCallRecord` 现在要求通话创建者并持久化 `RecordActive`、`RecordVideo`、可选标题和竖屏状态；重复设置返回 `GROUPCALL_NOT_MODIFIED`，`phone.getGroupCall` 通过 `record_video_active` 暴露当前控制状态。当前没有录制器、分段存储或转码消费者，因此这只代表控制面状态，不能证明有媒体文件生成。
- `messages.deletePhoneCallHistory` 不再把请求写入 KV 后返回空列表；它现在调用消息服务已有的 `msg.deletePhoneCallHistory`，透传 `revoke`、永久 auth key、pts 和受影响消息 ID，并对缺失 provider/nil 响应失败关闭。消息服务删除 DAO 也不再吞查询、逐会话删除、最后消息回读或 dialog 缓存更新错误。真实消息表删除、更新队列和撤回对端仍需 session/数据库验收。
- 消息历史 DAO 的普通会话、保存历史和未读提及读取现在传播 SQL 错误，拒绝 nil/非法 peer，并对频道返回明确的 enterprise/provider 错误；调用方不会再把数据库失败转换为空历史。频道历史仍需原生频道 provider。
- 当前 CSV 仍为 813 个唯一方法；按最新解析，静态状态为 320 项已知不完整、129 项仅路由、131 项组件部分实现、52 项部分隔离库、8 项隔离库、150 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装。生产验收状态为 331 项实现缺口阻塞、129 项未验收、125 项部分组件、47 项部分隔离库、8 项隔离库、152 项部分隔离 session、10 项隔离组件和 11 项传输包装；完整生产级验收仍为 0/813。
- 验证：`gofmt`、`git diff --check`、`go test -c ./app/bff/account/internal/core`、`go test -c ./app/bff/apifull/internal/core -o /tmp/apifull-core-final.test` 和 `go test -c ./app/service/biz/message/internal/dao -o /tmp/message-dao.test` 通过。APIFull runtime 测试仍因缺少 `APIFULL_MYSQL_DSN=127.0.0.1:13306/teamgram_audit` 未执行；没有连接生产数据库，也没有重启 `backend-backend-1`。

## 本轮第六十二次复核：Stars/礼物/群通话邀请台账纠偏

- 重新检查了 payment/gift/group-call 实现与台账：`payments.getStarsTopupOptions`、`payments.getStarsGiftOptions` 已读取持久化 Stars offer catalog（当前没有外部同步 provider）；`payments.getStarGifts`、`saveStarGift`、`convertStarGift`、`transferStarGift`、`getUniqueStarGift` 已有本地礼物目录/所有权的持久化路径；`phone.exportGroupCallInvite` 已生成绑定群通话、可消费的随机 bearer link。
- 上述方法从 `KNOWN_INCOMPLETE_STATIC` 调整为 `PARTIAL_COMPONENT_VERIFIED`，生产列仍为 `PARTIAL_COMPONENT_ONLY`，因为没有外部 Stars checkout/结算、礼物全局库存/拍卖/升级、通知深链投递或媒体控制面。`phone.toggleGroupCallRecord` 已降为控制状态的部分实现，实际录制和媒体消费者仍缺失。
- 台账仍为 813 个唯一方法：361 项已知静态缺口、151 项仅路由、105 项部分组件、27 项部分隔离库、10 项隔离库、138 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 371 项实现缺口阻塞、151 项未验收、99 项部分组件、22 项部分隔离库、141 项部分隔离 session、10 项隔离组件、8 项隔离库和 11 项传输包装。完整生产级验收仍未完成。

## 本轮第六十三次复核：用户主题目录与生产 r12 部署

- `account.getChatThemes` 现在读取用户已上传文档创建的主题，复用主题文档/access-hash 校验，并返回稳定 hash 与 `account.themesNotModified`；没有全局 Telegram chat-theme 或 unique-gift 目录时仍不伪造目录。隔离 MySQL 上的主题创建、读取、chat-theme 读回和 hash/not-modified 测试通过。
- 主机交叉编译 Linux ARM64 `bff`（`e6c2621787d1fb873c0c555305610b370f44b391a28edf7ff2054bbaf4cb4881`）和 `biz`（`287bd570c3f38475489a97300aa32cabd573b85dbd8fcddf660e9dbb76b9222e`），构建 `teamgram-server-latest:20261002-r12`，仅重建 `backend-backend-1`。部署后容器 `restart_count=0`，idgen/status/authsession/media/dfs/biz/msg/sync/bff/gnetway 全部运行；r11 镜像保留可回滚。
- 第一次打包发现工作树中的 `dfs` 是 macOS Mach-O，已从 r11 镜像恢复 Linux ARM64 DFS 后重打 r12；最终容器内 DFS SHA-256 为 `008138f991dae2cd56785aab74025cf2648fa95efb825e484a2ba95716b6fde5`，没有 `Exec format error`、panic 或 fatal 日志。
- 生产 MySQL 只读确认 `apifull_channel_event`、`apifull_channel_message`、`apifull_channel_message_seq` 存在且事件表仍为 0 行；没有执行迁移或写入业务测试数据。MySQL、Redis、Kafka、etcd、SeaweedFS 未因本轮重建而重启（Kafka 原有 restart count 保持 7）。生产未授权 WebSocket 探针返回 `Config` 且 `dc=2`。
- 台账 813 个方法中，`account.getChatThemes` 调整为 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`；全量生产级端到端验收仍未完成，短信/邮件 provider、真实支付结算、Passkey RP、第二 DC、原生频道完整 provider、Stories、通话媒体控制面和私密聊天跨设备密钥仍是阻塞项。

## 2026-10-02 本轮继续：频道输入状态同步

- `messages.setTyping` 的频道分支已接入 APIFull 频道成员表：先验证精确 `InputPeerChannel` access hash 和调用者成员身份，再过滤被踢成员，向其它活跃成员通过 Sync 推送 `updateChannelUserTyping`，并保留 `top_msg_id`。广播频道没有输入状态受众，继续返回 `METHOD_NOT_IMPL`；缺少频道 resolver 的最小部署也失败关闭。
- BFF 根服务的 `contactsChannelPlugin` 现在提供该 resolver；Linux ARM64 BFF 构建通过（`/tmp/bff-typing-r13`）。没有重启生产容器或向生产库写入频道输入状态，本轮不提升为 session 或生产验收。
- 跨多个 Sync 推送没有事务性：后续成员推送失败时，前面的成员可能已经收到更新。仍需隔离多用户 WebSocket 读回、广播频道拒绝和重启/生产验证。

## 2026-10-02 快捷回复消息发送/删除组件复核

- `messages.getQuickReplyMessages` 现在通过已有 message-service `MessageGetUserMessage` 回填保存的消息，不再在有快捷回复 ID 时直接 `METHOD_NOT_IMPL`；缺失、重复、非法或非本账号消息会失败关闭。
- `messages.sendQuickReplyMessages` 校验调用者拥有快捷回复消息，要求原消息是调用者发出的消息，克隆后改写目标 peer、发送者、时间和随机 ID，并通过现有 `MsgSendMessageV2` 写入；缺少消息/发送 provider 或返回 nil 时失败关闭。
- `messages.deleteQuickReplyMessages` 只删除调用者指定快捷回复下的消息关联，未知 ID、重复 ID 不会写入，成功后返回可编码的 `Updates`。
- `TestQuickReplySendUsesStoredMessagesAndMsgService`、`TestQuickReplyDeleteRemovesOnlyRequestedStoredMessages` 和既有快捷回复往返测试通过，使用隔离 APIFull 测试库。当前没有真实 session、跨进程重启或生产数据读回证据，因此三项台账只提升到 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`。

## 2026-10-02 快捷回复真实生产会话复核（session → BFF → msg → MySQL）

- 使用生产现有授权会话（用户 `136907714`）和现有基础群 `120032` 完成可逆的快捷回复流程。`messages.getQuickReplies` 返回 `messages.QuickReplies`，其中带有已保存消息；`messages.editQuickReplyShortcut` 返回 `BoolTrue`；`messages.getQuickReplyMessages` 返回 `messages.Messages` 并读回消息 `34`。
- `messages.sendQuickReplyMessages` 将消息 `34` 发到群 `120032`，返回 `Updates`，新消息 ID 为 `40`；随后通过 `messages.getHistory` 读回。`messages.deleteQuickReplyMessages` 返回类型化 `Updates` 并移除快捷回复关联，`messages.deleteQuickReplyShortcut` 返回 `BoolTrue`；最终再次读取快捷回复得到类型化空结果。临时群消息随后用显式消息删除清理，MySQL 读回该消息 `deleted=1`。
- 生产容器仍为 `teamgram-server-latest:20261002-r12`，容器 `restart_count=0`；本轮运行中的 BFF SHA-256 为 `384eab14c07b06cc24f6ac0111f3fcb2b8a16ac468ecf585b2ba271192d63056`，旧副本保留为 `/app/bin/bff.before-quickreply-20261002-070253`。只重启/替换 BFF 进程，msg、biz、session、gnetway、数据库、Redis、Kafka、etcd、DFS 均未重启。
- 恢复脚本遗留的 `apifull_kv` 空行 `qreply:136907714` 已按精确键和值删除；`qreply:msg:136907714` 保留为有效的 `[{"shortcut_id":0,"ids":[]}]`。没有写入任何凭据、验证码或会话密钥。
- 生产日志中的请求均完成于 session → BFF → msg/持久化链路，没有超时、500、panic 或 `METHOD_NOT_IMPL`。首次临时连接出现的 `invalid salt: 0` 是协议校正，不是业务失败。
- 台账现将上述 6 个方法提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`：`messages.getQuickReplies`、`messages.editQuickReplyShortcut`、`messages.deleteQuickReplyShortcut`、`messages.getQuickReplyMessages`、`messages.sendQuickReplyMessages`、`messages.deleteQuickReplyMessages`。`messages.reorderQuickReplies`、`messages.checkQuickReplyShortcut` 仍只有路由/组件证据，没有冒充真实业务验收。
- CSV 重新解析为 813 行且方法唯一：346 项明确静态缺口、149 项仅路由、111 项部分组件、27 项部分隔离库、8 项隔离库、149 项部分会话、10 项隔离组件、2 项 session 手动分发待验收和 11 项传输包装；生产列为 357 项实现缺口阻塞、149 项未验收、105 项部分组件、22 项部分隔离库、8 项隔离库、151 项部分会话、10 项隔离组件和 11 项传输包装。完整 Layer 229 生产级端到端验收仍为 **0/813**；短信/邮件 provider、支付结算、Passkey RP、第二 DC、原生频道完整 provider、秘密聊天跨设备密钥、礼物结算和通话媒体控制面仍按台账保持阻塞。
## 2026-10-02 本轮继续：Stars 余额与流水读回

- `payments.getStarsStatus`、`payments.getStarsTransactions` 和 `payments.getStarsTransactionsByID` 已接入 APIFull 的 MySQL Stars 账本。三条读取路径都按认证用户、peer、方向和分页校验，余额及交易 ID 只来自 `apifull_stars` / `apifull_star_tx`，未配置或没有账本记录时继续失败关闭。
- 新增 `TestStarsReadMethodsUseDurableLedger`：在 `127.0.0.1:13306/teamgram_audit` 写入一笔幂等 Stars 流水，再通过三个 Layer 229 handler 读回余额、流水和按 ID 查询，测试通过并清理随机用户行。没有连接生产库、没有给生产账户写入 Stars，也没有宣称外部 checkout/结算已完成。
- 台账仍为 813 个唯一方法；上述 3 项从 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP` 调整为 `PARTIAL_ISOLATED_DB_VERIFIED` / `PARTIAL_ISOLATED_DB_ONLY`。外部 Stars checkout、充值/退款 provider、权益授予、礼物库存同步和生产 session 验收仍保持阻塞。

## 2026-10-02 本轮继续：支付收据持久化读回

- `payments.getPaymentReceipt` 现在从 provider 已确认并落库的 `apifull_payment_receipt` 读取，先校验认证用户、peer 和消息 ID；没有已结算收据时继续返回明确错误，不把客户端凭据或本地请求状态当成收据。
- 新增 `TestPaymentReceiptReadbackUsesSettledLedger`：在隔离 `teamgram_audit` 通过支付状态机写入一笔已验证收据，再经 Layer 229 handler 读回交易号、币种、金额和标题，测试通过并清理随机用户行。外部 provider、权益授予和生产 session 仍未验收。
- 台账仍为 813 个唯一方法；该项从 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP` 调整为 `PARTIAL_ISOLATED_DB_VERIFIED` / `PARTIAL_ISOLATED_DB_ONLY`。

## 2026-10-02 本轮继续：会议通话广播链持久化

- `phone.sendConferenceCallBroadcast` 和 `phone.getGroupCallChainBlocks` 已接入已有 `apifull_group_call_conference` 控制记录：创建者必须通过真实 `InputGroupCall` access hash，广播块会替换持久化链头，读取按子链、offset 和 limit 校验，并返回 `updateGroupCallChainBlocks`。
- 新增 `TestConferenceBroadcastAndChainReadback`：在隔离 `teamgram_audit` 创建会议通话、写入广播块、通过 Layer 229 读回并验证错误 access hash，测试通过且测试通话清理完成。多块链、媒体 fanout、跨客户端同步和生产 session 仍未验收。
- 台账仍为 813 个唯一方法；上述两项从 `KNOWN_INCOMPLETE_STATIC` / `BLOCKED_BY_IMPLEMENTATION_GAP` 调整为 `PARTIAL_ISOLATED_DB_VERIFIED` / `PARTIAL_ISOLATED_DB_ONLY`。`phone.sendGroupCallEncryptedMessage`、RTMP 和 Stars 控制面仍保持失败关闭；录制仅有控制状态持久化，媒体消费者仍缺失。

## 2026-10-02 本轮继续：会议广播重放与投票统计读回

- `domain.UpdateConferenceCallBlock` 现在区分“同内容重放”和“通话记录不存在”：MySQL 对相同 block 返回 0 行变更时先确认记录仍存在，客户端重试保持幂等；未知记录仍返回 `sql.ErrNoRows`。`TestConferenceBroadcastAndChainReadback` 增加相同 block 重放断言。
- `stats.getPollStats` 已用 provider-backed poll fixture 验证完整读回：`TestPollStatsReadsPersistedBallots` 通过消息 provider 解析真实 poll、从持久化 ballot store 读取投票，并返回可编码的 `stats_PollStats` 与 `{"count":1}` 图表。生产 session、跨进程重启和完整 analytics provider 仍未验收。
- 本轮定向命令：`APIFULL_MYSQL_DSN=<isolated teamgram_audit DSN> go test ./app/bff/apifull/internal/core -run 'Test(PollStatsReadsPersistedBallots|ConferenceBroadcastAndChainReadback)$' -count=1`，通过；`gofmt` 与 `git diff --check` 通过。未连接生产数据库、未重启 `backend-backend-1`。
- CSV 重新计数为 813 个唯一方法：332 项已知静态缺口、141 项仅路由、118 项部分组件、40 项部分隔离库、8 项隔离库、151 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 343 项实现缺口阻塞、141 项未验收、112 项部分组件、35 项部分隔离库、8 项隔离库、153 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 2026-10-02 本轮继续：会议参与者更新完整性

- 修复 `phone.inviteConferenceCallParticipant` 的更新内容：邀请成功后广播现在带上完整的当前参与者集合，而不是只带新邀请者；`phone.deleteConferenceCallParticipants` 保持删除后的完整集合。新增 `TestConferenceParticipantInviteAndDeleteReadback`，覆盖 User provider access hash、持久化成员和邀请/删除后的 typed update。
- 定向隔离库命令 `APIFULL_MYSQL_DSN=<isolated teamgram_audit DSN> go test ./app/bff/apifull/internal/core -run 'TestConference(ParticipantInviteAndDeleteReadback|BroadcastAndChainReadback)$' -count=1` 通过；`go test -c ./app/bff/apifull/internal/core`、`gofmt` 和 `git diff --check` 通过。没有重启生产服务或写生产数据库。
- `phone.createConferenceCall`、`phone.inviteConferenceCallParticipant`、`phone.deleteConferenceCallParticipants` 现记为 `PARTIAL_ISOLATED_DB_VERIFIED` / `PARTIAL_ISOLATED_DB_ONLY`。CSV 重新计数：329 项已知静态缺口、141 项仅路由、118 项部分组件、43 项部分隔离库、8 项隔离库、151 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 340 项实现缺口阻塞、141 项未验收、112 项部分组件、38 项部分隔离库、8 项隔离库、153 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 2026-10-02 本轮继续：会议邀请拒绝持久化

- `phone.declineConferenceCallInvite` 现在通过消息 provider 解析邀请消息中的 conference call、校验 access hash 和当前参与者，再原子更新持久化成员并返回剩余成员的 typed update；未知消息、错误 call 或非参与者继续失败关闭。
- `TestConferenceParticipantDeclineRemovesInvitee` 与邀请/删除用例在隔离 `teamgram_audit` 通过；未连接生产数据库、未重启 `backend-backend-1`，真实邀请消息投递和跨客户端同步仍未验收。
- `phone.declineConferenceCallInvite` 现记为 `PARTIAL_ISOLATED_DB_VERIFIED` / `PARTIAL_ISOLATED_DB_ONLY`。CSV 重新计数：328 项已知静态缺口、141 项仅路由、118 项部分组件、44 项部分隔离库、8 项隔离库、151 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 339 项实现缺口阻塞、141 项未验收、112 项部分组件、39 项部分隔离库、8 项隔离库、153 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 2026-10-02 本轮继续：普通消息转发与媒体位置查询复核

- `messages.forwardMessages` 的普通用户/基础对话路径已有 provider-backed 实现：校验源/目标 peer、access hash、成员和原消息所有权，再通过 MsgSendMessageV2 写入；`messages.getSearchResultsPositions` 的用户/基础群媒体查询已有精确计数、分页排名和一致性校验。两者的频道、保存 peer、计划发送或媒体特化分支仍保持失败关闭。
- 既有 focused tests 继续通过：`TestMessagesForwardMessagesSendsStoredMessageAndReturnsUpdates`、发送失败边界，以及 `TestMessagesGetSearchResultsPositionsBuildsGlobalPhotoVideoPositions`、空结果/偏移/计数不一致/provider 错误用例。没有连接生产数据库或生产 session。
- 两项现记为 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`。CSV 重新计数：326 项已知静态缺口、141 项仅路由、120 项部分组件、44 项部分隔离库、8 项隔离库、151 项部分隔离 session、10 项隔离组件、2 项 session 分发待验收和 11 项传输包装；生产状态为 337 项实现缺口阻塞、141 项未验收、114 项部分组件、39 项部分隔离库、8 项隔离库、153 项部分隔离 session、10 项隔离组件和 11 项传输包装。完整生产级端到端验收仍为 **0/813**。

## 验证边界

- 本轮新增和相关定向用例均通过：APIFull conference/poll focused tests 使用隔离 `teamgram_audit`，messages forward/search focused tests 使用 provider stubs；构造器编译、唯一性计数和格式检查通过。
- `go build ./app/bff/bff/cmd/bff ./app/bff/apifull/...` 通过；本轮没有把该未部署构建产物替换到 `backend-backend-1`。
- `go test ./app/bff/apifull/internal/core -count=1` 的全包回归仍有既有失败：部分测试把 gRPC status 错误与原始 `mtproto` 错误直接比较，频道成员/删除、会议邀请、支付收据 fixture 还有相互独立的旧预期冲突，Stories/Stickers/Langpack 的 fail-closed 测试也有同类错误包装比较。该命令没有改变生产数据；这些失败不能被本轮 focused 通过替代，生产验收仍保持 **0/813**。

## 2026-10-02 主题/壁纸聊天应用路径复核（本轮）

- `messages.setChatTheme` 对带表情主题的请求现在明确经 `dialog.setChatTheme` 写入对话持久层；`messages.setChatWallPaper` 对调用者拥有的 DFS 壁纸经 `dialog.setChatWallpaper` 写入，并支持 `for_both` 与 `revert`。新增隔离组件测试检查用户、peer、主题值、壁纸 ID、双方标志和撤销清零请求，命令为 `APIFULL_MYSQL_DSN='root:<isolated-password>@tcp(127.0.0.1:13306)/teamgram_audit?parseTime=true' go test ./app/bff/apifull/internal/core -run '^TestChatThemeAndWallpaperUseDialogPersistence$' -count=1`，通过。
- 这两项从静态缺口调整为 `PARTIAL_COMPONENT_VERIFIED` / `PARTIAL_COMPONENT_ONLY`。测试使用 Dialog RPC stub，尚未连接真实 dialogs 服务做跨进程读回；主题 slug/唯一礼物主题和全局壁纸目录仍需权威 provider，故不能记为生产级完成。

## 2026-10-02 本轮继续：TTL 与在线人数错误传播复核

- `account.setAuthorizationTTL` 现在只接受协议允许的天数，并传播 User service 写入错误；User service 的 `users.authorization_ttl_days` 写入/读取也不再吞掉 DAO 错误。
- `messages.setDefaultHistoryTTL` 传播 User service 写入错误，`messages.getDefaultHistoryTTL` 对 provider 错误和 nil 响应失败关闭；`default_history_ttl` 的隔离库写入/读回通过。
- `messages.setHistoryTTL` 的 Dialog 事务现在检查两个方向的更新错误；隔离库验证了双方对话的 `ttl_period` 一起写入。
- `messages.getOnlines` 不再对缺失/非法 peer 返回伪造的 1 人在线；它要求群 peer，读取真实成员和最近 60 秒的 last-seen，并在依赖不可用时返回内部错误。聚焦测试覆盖计数、非法输入和 provider 失败。
- 定向命令：`APIFULL_MYSQL_DSN=<isolated teamgram_audit DSN> go test ./app/service/biz/user/internal/core ./app/service/biz/dialog/internal/core -run 'Test(UserDefaultHistoryTTL|UserAuthorizationTTL|DialogSetHistoryTTL)' -count=1`，以及四个相关 BFF/service core 包的 `go test`，均通过。没有重启 `backend-backend-1`，没有写生产数据库；完整 session 验收仍未完成。

## 2026-10-02 本轮继续：敏感内容设置权威存储

- `account.setContentSettings` / `account.getContentSettings` 现在通过 User service 的 `user_settings` 表读写 `sensitive_enabled`；写入失败不再返回 `BoolFalse` 加 nil error，读取失败和 BFF nil 响应也会失败关闭。
- 新增 BFF acknowledgement/nil-response 测试、User service 隔离 MySQL 写入/读回和断连错误测试，均通过。未重启 `backend-backend-1`，未写生产数据库；完整 session 流程仍待验收。

## 2026-10-02 本轮继续：Dialog 主题/壁纸持久化错误传播

- `dialog.setChatTheme` 现在检查双方对话行的事务更新并传播失败；`dialog.setChatWallpaper` 检查 SQL 和缓存失效结果，不再无条件返回 `BoolTrue`。
- 隔离 `teamgram_audit` 写入/读回验证了主题、壁纸 ID 和 override 标志，并用断连数据库验证两条失败路径。未重启生产容器，主题目录、全局壁纸目录和真实 session 更新仍未验收。

## 2026-10-02 本轮继续：搜索日历真实结果聚合

- `messages.getSearchResultsCalendar` 已确认走 canonical `MessagesSearch` provider；它不再依赖静态空结果，而是按真实消息日期聚合 period、消息数量、最小/最大消息 ID 和代表消息。
- 新增 handler 测试覆盖两天聚合、TL 编码和 provider 错误。频道/saved-peer 分支和部署后的 session 读回仍未验收。

## 2026-10-02 本轮继续：users.getUsers provider 错误边界

- `users.getUsers` 现在验证登录态和每个 `InputUser`，传播 User service 错误，拒绝 nil provider 响应，并返回 provider hydration 的 self/peer 用户；不会再把后端失败当成空向量或在 nil 上崩溃。
- 聚焦测试覆盖 self/peer 返回、非法输入和 provider 失败。access-hash 授权细节及部署后的 session 读回仍待验收。

## 2026-10-03 本轮继续：authsession 绑定与清理错误边界

- `authsession.bindTempAuthKey` 现在拒绝过短或不一致的外层/内层握手数据，要求外层 key 为永久 key，并通过 `BindTempAuthKeyV2` 在一个事务中锁定和更新永久/临时 key 两侧；临时 key 已绑定到其他永久 key 时返回 `TEMP_AUTH_KEY_ALREADY_BOUND`，任一持久化或缓存失效错误都不会再返回成功。
- `authsession.bindAuthKeyUser`、`unbindAuthKeyUser`、`resetAuthorization` 增加输入校验并传播 DAO 错误；`setAuthKey` 不再把写入失败吞成 `BoolFalse` 加 nil error。注销账户和确认手机号也会传播会话解绑、验证码缓存清理错误。
- 定向验证通过：
  `go test ./app/service/authsession/internal/core ./app/service/authsession/internal/dao ./app/service/authsession/internal/server/grpc/service ./app/bff/account/internal/core ./app/bff/authorization/internal/core -count=1`，以及 `git diff --check`。
  `TestAuthsessionResetAuthorizationAuditMySQL` 已用正确名称执行，但因未设置隔离 DSN 而跳过；没有连接生产数据库、没有重启或部署 `backend-backend-1`。
- 这些变更只覆盖会话/验证码错误传播和临时 key 原子绑定，不能替代手机号/短信 provider、Passkey RP、QR/多 DC、支付结算、原生频道 provider、礼物库存结算、通话媒体控制面或秘密聊天跨设备密钥；生产级 Layer 229 验收仍未完成。

## 2026-10-03 本轮继续：媒体资料照片持久化错误边界

- `media.uploadPhotoFile`、`media.uploadedProfilePhoto` 和 `media.uploadProfilePhotoFile` 现在拒绝 nil 的 DFS 响应，并传播照片尺寸、视频尺寸和主照片记录的写入错误；不会再返回一个客户端可见但数据库缺少尺寸/照片行的 `Photo`。
- 定向验证通过：`go test ./app/service/media/... ./app/bff/userchannelprofiles/... -count=1` 与 `git diff --check`。没有连接生产数据库、没有重启或部署服务。
- 这只修复现有媒体 provider 的错误传播；DFS 转码、跨 DC 媒体存储和生产 session 读回仍未验收。

## 2026-10-03 本轮继续：媒体文档落库错误传播

- `SaveDocumentV2` 现在返回 JSON 编码和文档插入错误，并拒绝空文档；贴纸、主题、壁纸、铃声及上传文档媒体路径会检查文档、缩略图和文档记录写入结果。
- 定向验证通过：`go test ./app/service/media/... -count=1` 与 `git diff --check`。没有连接生产数据库或替换运行中的服务。

## 2026-10-03 本轮继续：DFS 资料视频输入边界

- `dfs.uploadProfilePhotoFileV2` 现在允许仅上传视频的合法请求，拒绝 nil 请求/空文件，并按实际存在的 `video` 或 `file` 校验分片；此前仅视频请求会进入 nil 文件分支并在读取 `Parts` 时崩溃。
- `go test ./app/service/dfs/internal/core ./app/service/dfs/internal/dao ./app/bff/account/internal/core -count=1` 通过。尝试的 `go test ./app/service/dfs/...` 仍受仓库既有 JPEG 测试素材缺失和示例/ffmpeg 日志格式 vet 错误影响，未将其误记为本轮回归通过；`git diff --check` 通过。

## 2026-10-03 本轮继续：确认手机号验证码索引错误传播

- `account.sendConfirmPhoneCode` 现在检查按 `phone_code_hash` 建立验证码元数据索引的 Redis 写入结果；索引失败时返回内部错误，不再给客户端一个无法完成 `account.confirmPhone` 的假成功响应。
- 定向验证通过：`go test ./app/bff/account/internal/core ./app/bff/qrcode/... -count=1` 与 `git diff --check`。QR 仍只在单 DC、已有 Redis/authsession provider 下工作，未进行生产登录验收。

## 2026-10-03 本轮继续：消息删除输入和清理错误边界

- `messages.deleteMessages` 现在拒绝空/非法消息 ID，并拒绝消息服务返回的 nil 结果；`messages.deleteHistory` 对 nil 请求/响应失败关闭，并传播双方私聊 dialog 清理错误，避免消息已删除但客户端收到假成功。
- 定向验证通过：`go test ./app/bff/messages/internal/core ./app/bff/messages/internal/server/grpc/service -count=1` 与 `git diff --check`。频道删除仍依赖原生频道 provider，未提升为生产级全量验收。

## 2026-10-03 本轮继续：授权重置通知错误传播

- `account.resetAuthorization`、`auth.resetAuthorizations` 和账户注销现在检查 authsession 返回值及每条 `SyncUpdatesMe` 发布结果；更新通知失败时返回错误，不再宣称其它设备已被踢出。
- 定向验证通过：`go test ./app/bff/account/internal/core ./app/bff/authorization/internal/core -count=1` 与 `git diff --check`。这是更新投递边界修复，仍没有进行生产会话验收。

## 2026-10-03 本轮继续：验证码请求缺失 settings 边界

- `auth.sendCode` 和 `account.sendChangePhoneCode` 现在在读取 `CodeSettings` 前拒绝 nil 请求/缺失 settings，避免空参数触发 panic；有效请求统一使用安全 getter。
- 定向验证通过：`go test ./app/bff/authorization/internal/core ./app/bff/account/internal/core -count=1` 与 `git diff --check`。这只是输入边界修复，短信 provider、数据中心迁移和真实登录仍未验收。

## 2026-10-03 本轮继续：资料更新错误传播

- `account.updateProfile` 现在拒绝 nil 用户响应，传播 about/姓名写入错误，并只在用户服务写入成功后发布同步更新；同步发布失败也会返回错误，避免客户端看到更新成功但其它会话收到过期资料。
- 定向验证通过：`go test ./app/bff/userchannelprofiles/internal/core -count=1` 与 `git diff --check`。

## 2026-10-03 本轮继续：通知/TTL/联系人同步错误传播

- `account.updateNotifySettings`、`account.resetNotifySettings`、`messages.setHistoryTTL` 和 `contacts.deleteContacts` 现在检查 `SyncUpdatesNotMe` 结果；持久化成功但同步投递失败时返回错误，避免其它会话继续显示旧状态。
- 定向验证通过：`go test ./app/bff/dialogs/internal/core ./app/bff/notification/internal/core -count=1` 与 `git diff --check`。`contacts` 全包回归仍有工作树中既有的搜索 stub 预期失败，未把它误记为通过。

## 2026-10-03 本轮继续：联系人搜索授权边界与对话置顶同步错误

- `contacts.search` 不再调用没有用户范围证明的 `chat.search`；遇到基础群结果时明确失败关闭，避免把未授权群实体装入 `contacts.Found`。用户和频道结果仍要求权威 User/Channel provider 完整 hydration。
- `messages.toggleDialogPin` 增加认证、请求、provider 和 nil 响应校验；Dialog 写入、用户/群/频道实体 hydration 及 `SyncUpdatesNotMe` 的错误都会传播。频道缺少 resolver 时不再静默跳过实体。
- 定向验证通过：`go test ./app/bff/contacts/internal/core -count=1`、`go test ./app/bff/dialogs/internal/core -run 'TestMessagesMarkDialogUnread|TestMessagesGetPeerSettings|TestMessagesGetDialogs' -count=1` 和 `git diff --check`。未连接生产数据库、未重启或部署 `backend-backend-1`；基础群的用户范围搜索仍需补充专用 provider 后再恢复。
- `messages.sendScreenshotNotification` 同步本会话、私聊对端和群成员时现在传播 Sync/Chat provider 错误；频道没有成员控制面时明确返回 `METHOD_NOT_IMPL`，不再先写入同步再假报成功。
- `messages.saveDraft` 与 `messages.clearAllDrafts` 现在传播 Dialog、User/Chat/Channel hydration 和 Sync 错误，拒绝 nil 响应；频道没有 resolver 时失败关闭，不再伪造空频道实体。
- 定向验证通过：`go test ./app/bff/drafts/internal/core -count=1`、前述 dialogs/contacts 测试和 `git diff --check`。未连接生产数据库、未重启或部署 `backend-backend-1`。
- 包级回归 `go test ./app/bff/dialogs/... ./app/bff/drafts/... -count=1` 通过；这轮仍只验证代码与隔离 provider 边界，没有把结果提升为生产 session 验收。

## 2026-10-03 本轮继续：对话设置与消息置顶错误边界

- `messages.hidePeerSettingsBar` 在 User provider 确认删除后发布 `updatePeerSettings` 清空状态；认证、请求、nil provider 和 Sync 错误均失败关闭。新增测试覆盖 typed update 与同步失败。
- `messages.reorderPinnedDialogs` 改为同步完成实体 hydration 和 Sync 投递后再返回；用户、基础群、频道 resolver 缺失或返回空实体都会传播错误，不再使用后台 goroutine 吞掉失败。
- `messages.updatePinnedMessage` 增加认证、nil 请求、peer/provider/nil 更新校验；普通会话和频道继续使用现有 Msg/channelview 权威 provider。新增 provider error/nil-response/typed-result 测试。
- 验证通过：`go test ./app/bff/dialogs/internal/core -count=1`、`go test ./app/bff/messages/internal/core -run 'TestMessagesUpdatePinnedMessage' -count=1`、`go test ./app/bff/dialogs/... ./app/bff/drafts/... -count=1` 和 `git diff --check`。未连接生产数据库、未重启或部署 `backend-backend-1`。
- `contacts.block` / `contacts.unblock` 另外补上了 nil 请求、User/Sync provider、用户实体和 Bool 响应校验；现有 contacts 全包回归通过。
- `messages.getAllDrafts` 现在拒绝 nil Dialog 结果和 malformed draft entries，传播 User/Chat hydration 错误，并要求频道 resolver 返回完整实体；无 resolver 时返回 `METHOD_NOT_IMPL`，不再合成空频道。
- `messages.getSavedHistory` 同步补上了 MessageBox nil 检查、用户/群 hydration 错误传播和 channelview 频道实体回填，避免读历史时吞掉 provider 错误。
- `messages.getHistory` 补上了认证、输入、MessageBox/计数、用户/群 hydration 和频道实体校验；现有 channelview 频道历史路径保持不变，普通历史不再在 nil provider 结果上继续编码。
- `go test ./app/bff/messages/... -count=1` 通过；未连接生产数据库或部署运行中的服务。

## 2026-10-03 本轮继续：按 ID 消息读取错误传播

- `messages.getMessages` 现在在访问前校验消息 provider，拒绝 nil MessageBox/Message 和缺失 User/Chat provider，并传播实体读取失败；未知消息仍按协议返回 `messageEmpty`，频道实体没有权威 provider 时继续失败关闭。
- `messages.getRichMessage` 增加认证、peer/id 和 nil 响应校验；RichMessage 的用户/群实体 hydration 错误会返回原始 provider 错误，频道消息没有实体 resolver 时返回 `METHOD_NOT_IMPL`，不再静默返回不完整结果。
- `go test ./app/bff/messages/... -count=1` 通过；本轮未连接生产数据库、未重启或部署 `backend-backend-1`。

## 2026-10-03 本轮继续：对话/位置读取 provider 边界

- `messages.getPeerDialogs` 和 `messages.getPinnedDialogs` 现在拒绝未认证或 malformed 请求，传播 dialog、状态、通知设置、消息和用户/群实体 provider 的错误，并检查 nil 响应；频道实体沿用现有 `channelview` resolver，不再记录 enterprise 拦截后继续返回不完整结果。
- `messages.getSavedDialogs` 现在允许无 top message 的空 saved dialog，但对消息、用户、基础群和频道 hydration 做失败关闭校验；没有频道 resolver 时返回 `METHOD_NOT_IMPL`，不再合成空频道对象。
- `messages.getRecentLocations` 增加认证、依赖和 nil peer 边界，继续按历史页读取真实位置消息，并传播历史、用户/群实体和 nil provider 错误；频道因没有权威媒体位置存储继续明确返回 `METHOD_NOT_IMPL`。
- 验证通过：`go test ./app/bff/dialogs/... ./app/bff/messages/... ./app/bff/authorization/internal/dao ./app/bff/authorization/internal/core ./app/bff/account/internal/dao ./app/bff/account/internal/core -count=1` 与 `git diff --check`。本轮仍未连接生产数据库、未重启或部署 `backend-backend-1`，没有把组件测试提升为生产 session 验收。

## 2026-10-03 本轮子代理复核：验证码清理与礼物转移边界

- `auth.cancelCode` 及 account/authorization DAO 的 phone-code 删除现在校验规范化 hash 和 provider 状态；旧 hash 不会误删新挑战，登录清理使用规范化手机号。授权与账户 core/DAO 回归通过。
- `payments.transferStarGift` 在本地礼物账本没有消息/用户引用解析器或外部结算 provider 时，先拒绝这些引用和非规范 slug，再进入账本变更；新增消息引用拒绝测试。APIFull core 可编译，真实 MySQL 运行测试因本地 `127.0.0.1:13306` 不可用而未执行。
- 这些改动继续保持 fail-closed，不把本地账本当成外部支付、礼物库存或结算后端；短信 provider、真实支付和生产 session 验收仍未完成。

## 2026-10-03 本轮继续：chatlist 邀请一致性边界

- `chatlists.exportChatlistInvite` 和 `editExportedInvite` 现在在写入邀请前验证 User/Chat/Channel 实体；缺少对应 provider 时失败关闭。索引写入失败会回滚已追加的邀请，避免产生不可查询的半条记录。
- `chatlists.deleteExportedInvite`、`editExportedInvite` 拒绝 nil/空 slug；`chatlistEntities` 对缺失 User/Chat provider 返回内部错误，不再发生 nil 解引用。`requireUserId` 也覆盖 nil core。
- APIFull core 编译通过。完整 chatlist 生命周期测试已补上，但当前环境 `APIFULL_MYSQL_DSN` 未配置且 `127.0.0.1:13306` 不可用，TestMain 无法启动隔离数据库，因此本轮没有把该生命周期标成已验收。

## 2026-10-03 本轮继续：保存对话与二维码错误边界

- 实际注册的 `RPCSavedMessageDialogs` 路由补上了 `messages.getSavedDialogsByID`、`messages.getPinnedSavedDialogs` 的认证、请求/依赖校验、provider 错误传播和实体 hydration；频道实体只由持久化 `channelview` resolver 提供，不再合成空频道。`messages.reorderPinnedSavedDialogs` 拒绝 nil/未认证请求并检查下游返回值；`messages.deleteSavedHistory` 在校验 peer 后明确返回 `METHOD_NOT_IMPL`。保存历史删除仍没有消息服务删除 RPC，因此不把“查询后返回 affected history”当成删除成功。
- QR 登录接受流程现在检查 `SyncUpdatesMe` 的返回值；过期 token 清理失败也会向上返回，避免 authsession/Redis 状态已变更但客户端收到假成功。该修复不增加多 DC token migration 或受信任 app registry。
- 定向验证通过：`go test ./app/bff/savedmessagedialogs/... -count=1`、`go test ./app/bff/qrcode/... -count=1`、`go test ./app/service/authsession/internal/core ./app/service/authsession/internal/dao ./app/service/authsession/internal/server/grpc/service -count=1`、`go test ./app/bff/dialogs/internal/core ./app/bff/messages/internal/core -count=1` 和 `git diff --check`。未连接生产数据库、未重启或部署 `backend-backend-1`；保存对话真实 session、删除保存历史、原生频道保存历史、多 DC QR 和完整 Layer 229 生产验收仍未完成。

## 2026-10-03 本轮继续：隔离镜像重建、消息收件人落库与并发登录

- 用最终工作树重建并强制重建隔离 `teamgram` 容器；随后删除并重新创建本轮隔离 MySQL/Redis/Kafka/etcd/MinIO 卷。生产 `backend-backend-1`、生产数据库和其它生产服务未修改。
- `authsession.BindAuthKeyUser` 对 MySQL deadlock/lock-wait（错误码 1213/1205）增加最多三次、带上下文取消的短退避重试。此前十四个虚拟用户并行登录会偶发 `auth.SignIn` 500；修复后并发登录流程不再因该事务性冲突失败。`go test ./app/service/authsession/internal/dao` 通过。
- 复跑真实隔离 WebSocket → session → BFF 流程：`probe-message-read-search.ts` 全部通过（私聊/群消息、搜索、已读、mention 清理、非法参数）；`probe-poll-flows.ts` 全部通过（投票读回、投票、结果、公开投票者、阅读数和权限拒绝）；`probe-scheduled-message-lifecycle.ts` 全部通过（创建、查询、取消、立即投递、收件人读回和参数拒绝）。Kafka `Inbox-MainCommunity-S` 三个分区当前 lag 为 0；隔离 MySQL `messages` 与 `user_pts_updates` 均有最新业务记录。
- `verify-business-flows.sh` 在本轮三条定向探针后因 fixture 手机号触发 `PHONE_NUMBER_FLOOD` 停止，不能把十四用户全业务流程再次记为通过；该 400 是登录频控，非实现缺口。此前已通过的隔离业务流证据继续保留，但不替代本轮重新登录验收。
- CSV 结构化解析确认仍为 813 行、8 列、方法名唯一：296 项 `KNOWN_INCOMPLETE_STATIC`、129 项仅路由、155 项部分组件、51 项部分隔离库、151 项部分隔离 session、10 项隔离组件、8 项隔离库、2 项 Layer 229 手动分发、11 项传输包装；生产列为 307 项实现缺口阻塞、129 项未验收、149 项部分组件、46 项部分隔离库、153 项部分隔离 session、10 项隔离组件、8 项隔离库和 11 项传输包装。完整生产级 Layer 229 验收仍为 0/813。

## 2026-10-03 大文件分片上传门禁

- `upload.saveBigFilePart` 现在在进入 DFS 前校验认证上下文、请求非空、`file_id`、分片索引范围、总分片数和非空 payload；合法请求转发 `big=true` 及总分片数，DFS 错误原样传播。
- `TestUploadSaveBigFilePartValidatesEnvelope` 和 `TestUploadSaveBigFilePartForwardsBigPartAndProviderError` 通过。该项提升为 `PARTIAL_COMPONENT_VERIFIED`，隔离 session、对象存储重启和生产验收仍未完成。

## 2026-10-03 本轮继续：Passkey TL 包装与隔离 ceremony

- Passkey 的登录和注册 options handler 原先直接构造 `DataJSON`，没有设置 TL predicate。BFF 日志虽显示 options 已生成，session 编码只写出外层 `auth.passkeyLoginOptions` 构造器，GramJS 读取内层对象时会断流。现统一使用 `MakeTLDataJSON(...).To_DataJSON()`，确保 `DataJSON` 按 Layer 229 编码。
- `teamgramd/etc2/bff.yaml` 为隔离栈增加明确的 HTTPS RP、fixture API allowlist；生产 `etc/bff.yaml` 保持未配置。`docker/web-isolated/probe-passkey-options.ts` 通过真实 WebSocket/session/BFF/authsession 路径验证登录与注册 challenge/RP、`account.getPasskeys` 类型化空列表和未知凭证 `BoolFalse`。隔离 MySQL 回读到 login/registration ceremony 行。
- `go test ./app/bff/passkey/...`、`git diff --check` 和隔离 Passkey 探针通过。注册 attestation、完成登录、公钥凭证与生产 RP/真实认证器仍未验收，不能将 Passkey 生产缺口标记为完成。
- 客户端 GramJS 要求 WebAuthn `userHandle` 解码为 `dc_id:user_id`；服务端 `WebAuthnID()` 已统一生成该格式，完成登录时校验句柄 DC 与本地 DC 一致，避免真实认证器 assertion 在客户端被误判为无效或绑定到错误数据中心。隔离探针还校验注册 options 的 user.id 解码为当前 DC 和用户。`TestPasskeyUserHandleUsesClientDcFormat` 覆盖格式、解析和非法句柄拒绝。
- `auth.finishPasskeyLogin` 现在拒绝没有成对 `from_dc_id` 的 `from_auth_key_id`，并拒绝零迁移 auth key；`TestPasskeyFinishRejectsUnpairedMigrationAuthKey` 覆盖该边界。独立第二 DC 的 auth key 导入和跨 DC Passkey 登录仍未实现，继续保持 fail-closed。
- Passkey 输入现在要求 `id` 与 `raw_id` 都是非空 Raw-URL base64 且解码字节完全一致；混淆凭证包在进入 WebAuthn 解析前返回 `AUTH_TOKEN_INVALID`。`TestPasskeyCredentialIDsMustMatch` 覆盖拒绝与合法匹配路径。

## 本轮收口统计（2026-10-03）

- 台账保持 813 个唯一方法、8 列；当前审计状态为 `KNOWN_INCOMPLETE_STATIC=292`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=128`、`PARTIAL_COMPONENT_VERIFIED=152`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、`PARTIAL_SESSION_VERIFIED=155`、`ISOLATED_COMPONENT_VERIFIED=10`、`ISOLATED_DB_VERIFIED=8`、手动分发待验收 2、传输包装 11。
- 生产验收状态为 `BLOCKED_BY_IMPLEMENTATION_GAP=299`、`NOT_ACCEPTED=128`、`PARTIAL_COMPONENT_ONLY=150`、`PARTIAL_ISOLATED_DB_ONLY=50`、`PARTIAL_ISOLATED_SESSION_E2E=157`、隔离组件 10、隔离库 8、传输包装 11；完整生产级 Layer 229 验收仍为 `0/813`。
- 生产 `backend-backend-1` 仍为 `teamgram-server-latest:20261002-r12`，启动时间 `2026-10-03T00:53:16Z`，重启计数 0；本轮没有连接、写入或重启生产数据库和生产服务。

## 2026-10-03 本轮继续：频道管理审计真实读回

- `channels.getAdminLog` 已在隔离 `teamgram_audit` 中完成合法请求闭环：创建频道、加入成员、读取持久化审计事件、返回频道和用户实体，并按 Layer 229 编码成功。此前仅有 nil 参数 fail-closed 覆盖，现新增 `TestChannelsGetAdminLogRoundTrip`。
- 为执行数据库测试创建的审计库和共享网络代理只存在于隔离 Docker 网络；生产 `backend-backend-1`、生产数据库和其它生产服务没有连接、写入或重启。
- 本轮没有把该方法提升为生产级通过；原生频道 provider、跨会话更新和生产 session 仍未验收。完整 Layer 229 生产级验收仍为 `0/813`。

## 2026-10-03 本轮收口：隔离镜像加载最新工作树

- 使用本机 Go 1.23.12 构建了最新隔离 `teamgram` 镜像并重启隔离 `teamgram` 容器；MySQL、Redis、Kafka、etcd、MinIO 数据卷保留，`teamgram_audit` 仍可读。隔离 Teamgram、MySQL 均为 running/healthy。
- 生产 `backend-backend-1` 仍为 `teamgram-server-latest:20261002-r12`，`restart=0`，启动时间不变；没有连接、写入或重启生产数据库和生产服务。
- 最新源码定向测试、隔离 MySQL 频道设置/审计/不活跃频道回读和 Layer 229 编码均通过；GramJS 真实登录探针因当前主机未提供 `TEAMGRAM_GRAMJS_DIR` 未重跑，不能把隔离 session 结果提升为生产验收。

## 2026-10-04 本轮继续：真实 GramJS 多用户业务流复验

- 使用 `/Users/mac/open/test/telegram-tt-master` 的 GramJS runtime 连接隔离网关 `127.0.0.1:31443`，14 个隔离 fixture 账号全部完成 `auth.SignIn`，没有 `PHONE_NUMBER_FLOOD`、超时或连接断开。
- `docker/web-isolated/verify-business-flows.sh` 完整通过（`TEAMGRAM_FLOW_TAG=continue-20261003`）：联系人搜索、私聊消息、联系人导入/保存、未注册号码归属、屏蔽用户、群组成员和反应、频道发信/取历史/已读/阅读数/搜索/差分/详情、频道群通话控制、频道统计均完成真实 session → BFF → service → MySQL/Kafka 闭环。
- 该证据只覆盖当前隔离镜像和隔离数据库，不改变生产 `backend-backend-1`，也不证明真实短信/邮件 provider、支付结算、第二 DC、原生频道存储、礼物库存或通话媒体控制面已具备；完整 Layer 229 生产级验收仍为 0/813。

## 2026-10-04 本轮继续：频道帖子搜索的 canonical 子集

- `channels.searchPosts` 现在复用 APIFull canonical 频道消息表，校验频道 access hash 和成员身份，支持本频道文本搜索、`hashtag`（自动规范化为 `#tag`）、`offset_id` 和 limit，并返回带频道实体的 typed `messages.Messages`。`offset_rate` 仅作为本频道 SQL 行偏移，不能代表跨频道排名；付费 Stars 与 bot 搜索仍明确拒绝。全局帖子索引、完整 Layer 229 游标语义和生产 session 验收仍缺。
- `searchPostsQuery` focused test 通过；`TestHistoryAndEditDataCheckInputPeerAuthorization` 在隔离 MySQL 中新增 hashtag canonical message 回读并通过，覆盖错误 access hash 与非成员拒绝。该项只提升为 `PARTIAL_COMPONENT_VERIFIED`，没有把局部频道搜索冒充 Layer 229 全语义或生产验收。
- 本轮未连接、写入或重启生产 `backend-backend-1`；原生频道 provider、跨频道分页、实时 session 和生产验收仍未完成。

## 2026-10-04 当前台账计数

- `LAYER229_METHOD_LEDGER.csv` 结构仍为 813 行、813 个唯一方法、8 列。当前审计状态为 `KNOWN_INCOMPLETE_STATIC=288`、`PARTIAL_COMPONENT_VERIFIED=155`、`PARTIAL_SESSION_VERIFIED=155`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=128`、`PARTIAL_ISOLATED_DB_VERIFIED=56`、`ISOLATED_COMPONENT_VERIFIED=10`、`ISOLATED_DB_VERIFIED=8`、传输包装 11、session 手动分发 2。
- 生产验收列为 `BLOCKED_BY_IMPLEMENTATION_GAP=296`、`PARTIAL_ISOLATED_SESSION_E2E=157`、`PARTIAL_COMPONENT_ONLY=153`、`NOT_ACCEPTED=128`、`PARTIAL_ISOLATED_DB_ONLY=50`、`ISOLATED_COMPONENT_ONLY=10`、`ISOLATED_DB_ONLY=8`、传输包装 11；完整生产级 Layer 229 验收仍为 `0/813`。
- 频道邀请路径补上权限优先检查：非管理员在缺少 User provider 时也稳定返回 `CHAT_ADMIN_REQUIRED`，不会被错误转换成 500；`TestChannelsMemberLifecycleRoundTrip` 在隔离 `teamgram_audit` 通过。
- Stars 礼物选项在没有 offer provider 时现在先返回 `METHOD_NOT_IMPL`，不会对空请求错误宣称为构造器问题；Stories album create 也先做认证再校验请求。APIFull core 编译通过，真实支付/故事 provider 仍未配置。

## 2026-10-04 本轮继续：生产 backend-backend-1 真实链路验收

- 从当前工作树交叉编译 Linux ARM64 的 `authsession`、`bff`、`biz`、`gnetway`、`idgen`、`media`、`msg`、`session`、`status`、`sync`，以生产现有 `teamgram-server-latest:20261002-r12` 为运行时基础生成 `teamgram-server-latest:20261004-r1`；`dfs` 因 WebP/C 依赖无法无损交叉编译，保留生产现有二进制。通过 Compose 仅重建 `backend-backend-1`，未重启或修改 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn 或 identity 网络服务。
- 新容器 `backend-backend-1` 已运行，镜像为 `teamgram-server-latest:20261004-r1`，11 个 Teamgram 进程均已启动并在生产 etcd 注册；BFF/session/gnetway 日志确认连接生产 MySQL `teamgram`、Redis、Kafka 和对象存储配置。容器 `restart_count=0`（Compose recreate 后为新容器计数），数据库未执行迁移、清库或 schema 改动。
- 使用生产 `auth_keys` 中用户 `136907713` 的现有 auth key，通过真实 GramJS WebSocket `127.0.0.1:11443` 完成认证链路：`getMe → User`、`help.getConfig → Config`、`messages.getDialogs → messages.DialogsSlice`，返回均为类型化结果。认证 key 只在本地探针进程中读取，未写入台账或日志。
- 发送唯一 `prod-e2e-*` 自对话消息返回 `Updates`；在生产 `messages` 表按 `message` 精确读回 `user_id=136907713`、`random_id`、`dialog_message_id` 和 `user_message_box_id`，证明 session → BFF → msg/biz → MySQL 写入闭环。随后通过真实 `messages.deleteMessages` 清理，目标行 `deleted=1`，未留下可见测试消息。
- 同一真实生产账号的 `auth.sendCode` 已验证 provider 边界：合法存在的手机号进入 BFF/provider 后返回 `SMS_CODE_CREATE_FAILED`，不是客户端或网关超时；当前生产 `Code.Name=none`、SMS/Email endpoint 为空，因此短信/邮件登录、真实新会话注册仍不能标为生产通过。
- 本轮生产证据只覆盖一个已有 auth key 的只读与可清理自对话写入链路；没有宣称 813 个 Layer 229 方法生产通过。真实短信/邮件 provider、支付/礼物结算、第二 DC、原生频道存储、Passkey 真实认证器、秘密聊天跨设备密钥和通话媒体控制面仍是实现或配置阻塞。

## 2026-10-04 本轮继续：登录验证码失效错误边界

- `account.invalidateSignInCodes` 现在校验请求、永久授权密钥和缓存依赖；已认证调用会先解析当前手机号，再只删除与请求验证码或 hash 匹配的手机号/hash 别名。用户服务、缓存读取和缓存删除失败都会向上传播，不再在依赖不可用或删除失败时返回假 `BoolTrue`。
- 新增授权 core 测试覆盖别名清理、缓存读取/删除失败、nil 依赖和用户 provider 错误。当前工作树整体测试仍被其他代理在 `app/bff/apifull/internal/core/secret_chats.go` 的既有 `int32`/`int64` 编译错误阻断，本轮未把它记为通过；`git diff --check` 待整体工作树修复后复跑。
- 该项仅提升为组件级证据，未连接生产 Redis/user service、未进行完整登录 session 或重启持久性验收；生产级 Layer 229 验收仍为 `0/813`。当前 CSV 为 813 行、813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=287`、`PARTIAL_COMPONENT_VERIFIED=157`、`PARTIAL_SESSION_VERIFIED=158`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=125`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、隔离组件 10、隔离库 8、传输包装 11、手动分发 2；生产列中 `BLOCKED_BY_IMPLEMENTATION_GAP=295`、`NOT_ACCEPTED=125`、`PARTIAL_COMPONENT_ONLY=154`、`PARTIAL_ISOLATED_DB_ONLY=49`、`PARTIAL_ISOLATED_SESSION_E2E=161`，其余为隔离/传输项。

## 2026-10-04 本轮继续：密码重置拒绝状态持久化

- `account.declinePasswordReset` 已有真实的 caller-scoped password-state 读写路径：认证后加载共享密码状态，设置 `ResetDeclined` 并保存，保存失败会向上传播，不再把未持久化状态报告为成功。
- 新增 APIFull focused tests 覆盖拒绝状态读回和 password-state store 错误传播。整体 APIFull 测试仍受共享工作树 `app/bff/apifull/internal/core/secret_chats.go:243` 的 `int32`/`int64` 编译错误阻断，本轮没有把它记为全包通过；`git diff --check` 通过。
- 该项仅提升为组件级证据，未完成真实 Redis/数据库、完整 reset wait/recovery session、重启持久性或生产验收。当前 CSV 为 813 行、813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=287`、`PARTIAL_COMPONENT_VERIFIED=158`、`PARTIAL_SESSION_VERIFIED=158`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=124`、`PARTIAL_ISOLATED_DB_VERIFIED=55`、隔离组件 10、隔离库 8、传输包装 11、手动分发 2；生产列中 `BLOCKED_BY_IMPLEMENTATION_GAP=295`、`NOT_ACCEPTED=124`、`PARTIAL_COMPONENT_ONLY=155`、`PARTIAL_ISOLATED_DB_ONLY=49`、`PARTIAL_ISOLATED_SESSION_E2E=161`，其余为隔离/传输项。

## 2026-10-04 本轮继续：Passkey provider 与 auth key 边界

- Passkey provider 现在拒绝 ICANN 公共后缀作为 RP ID（例如 `com`、`co.uk`），避免错误配置把不属于本服务的子域 origin 纳入允许范围；隔离 `passkey.teamgram.local` 配置仍可通过。
- `auth.finishPasskeyLogin` 对 `from_auth_key_id` 及 metadata 回退 auth key 只接受正数；缺少配对 `from_dc_id`、零值和负值均在 assertion 验证前失败关闭。`go test ./app/bff/passkey/... -count=1` 与 `git diff --check` 通过。
- 这些是配置与输入边界修复，未连接生产 Passkey RP、真实认证器或第二 DC，Passkey 注册/登录生产验收仍保持阻塞。

## 2026-10-04 本轮继续：生产双客户端秘密聊天 DH 探针

- 在 `backend-backend-1` 上仅热替换 BFF，保留旧二进制备份；容器保持 `running`、`restart_count=0`，MySQL、Redis、Kafka、etcd、session、msg、biz 等其它服务没有重启。新 BFF 的 SHA-256 为 `a8dab4df7c60291e20df32f7a3a5bf7394771abda34e5c62e7344318d4f3788b`，启动日志显示 APIFull MySQL 已打开。
- 使用生产库现有用户 `136907713` / `136907714` 的真实 GramJS auth key，通过 `127.0.0.1:11443` 完成 `messages.getDhConfig`、2048-bit DH 公钥和共享密钥相等性、SHA-1 fingerprint、`messages.requestEncryption`、离线等待后 `messages.acceptEncryption`、第二 Bob 设备接受与旧设备重试保护、双方重连、`messages.sendEncrypted`、`updates.getDifference`、`messages.readEncryptedHistory` 和 `messages.discardEncryption`。失败尝试留下的 waiting 会话随后也通过真实 `discardEncryption` RPC 清理。
- 生产 MySQL 对成功 chat 记录出示 `g_a`/`g_b` 各 256 字节、最新 fingerprint、Alice 一行和 Bob 两行 signed auth-key device rows（均 epoch 1）；保留历史期间查到两条 32 字节加密消息，后一条的 `read_at` 已写入。最终所有 7 条本次测试会话均为 `state=discarded`、`history_deleted=1`，消息表无残留。底层校验已统一接受非零 signed auth key ID，新增回归覆盖负数设备 ID。
- 这次证明的是 DH 协商、离线握手恢复、跨设备密钥持久化/重试保护、QTS 差分投递、加密 payload 发送/读历史和销毁的生产会话链路。探针发送的是随机 32 字节 payload，没有实现 Telegram Secret Chat 的 AES-IGE key derivation/明文封装，因此不能把“客户端可解密的端到端消息”标记为通过；`messages.sendEncryptedFile`、`messages.sendEncryptedService`、`messages.receivedQueue`、密钥轮换 epoch >1 和真实客户端 UI 仍未验收。完整 Layer 229 生产级验收继续为 **0/813**。

## 2026-10-05 本轮继续：Poll TL 返回崩溃与生产全链路复验

- 全量真实探针第一次在 `messages.getUnreadPollVotes` 暴露了服务端编码缺口：返回的 `MessageMediaPoll.Results` 为 nil，session 在 Layer 229 编码 `messages.Messages` 时触发 `PollResults.Encode` 空指针，客户端表现为连接等待。`pollMessage` 现返回完整 `Poll`（非零 `id/hash`、问题、答案）和非空 `PollResults`（答案选项、零投票数）；新增 `TestPollMessageEncodingIncludesCompletePoll` 覆盖字段和 Layer 229 编解码无剩余字节。
- 从当前工作树构建并部署 `teamgram-server-latest:20261005-r11-prod`，Compose 只重建 `backend-backend-1`；生产 MySQL、Redis、Kafka、etcd、SeaweedFS、coturn、identity 网络和其它容器未重启。最终容器 `running`、`restart=0`、`oom=false`，11 个进程均已启动并重新注册。
- 使用生产库已有用户 `136907713`/`136907714` 的 auth key，通过 `127.0.0.1:11443` 真实 GramJS WebSocket 完成：`getMe`、`help.getConfig`/`getNearestDc`/国家/时区、对话；私聊发送/历史/本地与全局搜索/已读/编辑；反应设置/读取/清除；公开投票发送、双方投票、结果、投票者、追加答案、未读投票；基础群论坛话题、消息、详情、标题读回；定时消息创建/读取/按 ID/取消/立即投递；canonical 频道发送/历史/搜索/按 ID、频道已读、内容已读、阅读数、差分、编辑、删除和删除后的 `MESSAGE_ID_INVALID`。所有结果均为预期 typed constructor，无 400 误报、500、超时或连接断开。
- 探针 tag 为 `prod-chain-1791148268761-53064`；生产库精确查询确认可见 `messages=0`、`apifull_channel=0`，仅保留一个 `deactivated=1` 的基础群删除墓碑。通话控制面和可逆账户/通知/联系人/设备/草稿/置顶探针也在 r10 上通过，临时消息和 `apifull_group_call` 均清理为 0。
- 短信 provider 的兼容修复保留 `Name=me` 的历史 GET 查询契约，并让显式 `http` 使用 JSON POST；`pkg/code/...` focused tests 通过。QR 包全套 focused tests 通过；支付 provider 的金额/币种绑定和 slug 账本幂等边界已补上定向测试。APIFull runtime 全包仍因本机 `127.0.0.1:13306/teamgram_audit` 隔离 MySQL 端口不可达而未运行，未连接生产库做测试。
- 当前 CSV 仍为 813 行、813 个唯一方法：审计状态 `KNOWN_INCOMPLETE_STATIC=284`、`PARTIAL_COMPONENT_VERIFIED=164`、`PARTIAL_SESSION_VERIFIED=169`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_VERIFIED=54`、隔离组件 10、隔离库 8、session Layer 229 手动分发 2、传输包装 11；生产列为 `BLOCKED_BY_IMPLEMENTATION_GAP=292`、`PARTIAL_ISOLATED_SESSION_E2E=177`、`PARTIAL_COMPONENT_ONLY=156`、`NOT_ACCEPTED=111`、`PARTIAL_ISOLATED_DB_ONLY=48`、隔离组件 10、隔离库 8、传输包装 11。完整生产级 Layer 229 仍不能标记为 813/813。

## 2026-10-06 本轮继续：生产数据库 auth key 的只读 typed 链路

- 浏览器 Local Storage 中的两个旧 auth key 均不在当前生产 `auth_keys` 表，直接使用它们会在 session 层返回 `AUTH_KEY_UNREGISTERED` 并关闭连接；没有修改或迁移这些旧会话。
- 只读读取生产 `auth_keys` 中已登记的用户 `136907713` 永久 key（密钥只存在探针进程环境中，不写入仓库、台账或日志），通过 `127.0.0.1:11443` 完成真实 DC2 WebSocket → gateway → session → BFF → 生产数据库链路。`help.getConfig` 返回 `Config`，`photos.getUserPhotos` 返回 `photos.Photos`（photosCount=0），`account.getBusinessChatLinks` 返回 `account.BusinessChatLinks`（linksCount=0）。
- `docker/production/probe-current-browser-session.ts` 现在会去重候选 key、记录非敏感 auth key ID、支持外部注入已登记 key、使用原始 TL 构造器补齐客户端运行时缺失的 `account.getBusinessChatLinks`，并跳过不属于本次主 DC 验收的 media sender；探针单次运行退出码为 0。
- 因此 `photos.getUserPhotos` 与 `account.getBusinessChatLinks` 从 `NOT_ACCEPTED` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。前者仍没有生产非空照片分页证据，后者没有 create/edit/delete 或重启持久性证据。
- 本轮没有重启 `backend-backend-1`、数据库或其它服务，也没有执行写入或破坏性方法。当前生产列按 CSV 结构化解析为：`BLOCKED_BY_IMPLEMENTATION_GAP=290`、`NOT_ACCEPTED=61`、`PARTIAL_COMPONENT_ONLY=176`、`PARTIAL_ISOLATED_DB_ONLY=46`、`PARTIAL_ISOLATED_SESSION_E2E=211`、隔离组件 10、隔离库 8、传输包装 11；完整生产级 Layer 229 仍为 `0/813`。

## 2026-10-06 生产双用户全量探针续验（backend-backend-1）

- 使用生产 MySQL 中已登记的用户 `136907713` / `136907714` 永久授权，仅在单次本地探针进程环境变量中注入；未打印、写入仓库或写入台账。目标为 `backend-backend-1`，容器仍为 `running`、`restart=0`、`oom=false`，镜像 `teamgram-server-latest:20261005-r13-prod`；当前 BFF `/app/bin/bff-r16` SHA-256 为 `baf631e8a9b23055579458f337d1eaf38d4fbab184afe7e088e6029f8f289e68`。
- `probe-real-chain.ts` 真实完成认证、配置/国家/时区、对话、私聊发送/历史/本地与全局搜索/已读/编辑、反应、公开投票与投票读写、basic group 论坛/消息/详情/标题、定时消息和 canonical 频道成员/详情/消息/搜索/已读/内容已读/阅读数/差分/编辑/删除。返回类型包括 `Config`、`NearestDc`、`help.CountriesList`、`help.TimezonesList`、`messages.DialogsSlice`、`messages.Messages`、`messages.ForumTopics`、`messages.ChatFull`、`messages.MessageViews`、`updates.ChannelDifference` 等；删除后按 ID 读取得到预期 `MESSAGE_ID_INVALID`。
- `probe-reversible.ts` 完成 TTL、通知设置、联系人增删、生日列表、设备注册/注销、outbox 已读日期、消息置顶/取消、草稿和对话置顶/取消；`probe-library-reversible.ts` 完成 Saved GIF 写入/读回/清理以及 effects/recent-reactions typed 空结果；`probe-multi-media.ts` 完成两项联系人媒体 grouped 写入、双方读回和删除；`probe-business-profile.ts` 完成五项业务资料写入、MySQL 读回和空值清理；`probe-forum-topics.ts` 完成论坛开关、话题创建/编辑/置顶/排序和删除频道。
- `probe-chat-reactions.ts` 完成真实群可用反应写入、`messages.getFullChat` 读回、MySQL UTF-8 读回和清理；`probe-reactions-focused.ts` 完成真实反应、未读/已读/清空；`probe-delete-history-focused.ts` 完成真实群消息和 `messages.deleteHistory` 后的发送方/接收方历史清理。`probe-control-plane.ts` 在补齐 B 用户 access hash 后完成 `phone.createGroupCall`、标题编辑、读取、邀请、加入、离开和销毁，返回 `Updates` / `phone.GroupCall`。
- 生产 MySQL 清理核对：本轮 tag 的 active `messages` 为 `0`；当前探针创建的群已 `deactivated=1`；当前通话创建记录在销毁后不再保留。频道与论坛临时对象均完成删除。工作树台账仍为 813 行、813 个唯一方法；本次直接补验的方法已写入 `LAYER229_METHOD_LEDGER.csv`，生产列当前计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=297`、`PARTIAL_ISOLATED_SESSION_E2E=201`、`PARTIAL_COMPONENT_ONLY=154`、`NOT_ACCEPTED=89`、`PARTIAL_ISOLATED_DB_ONLY=43`，另有隔离组件 10、隔离库 8、传输包装 11。
- 最近 45 分钟生产日志没有 `panic`、`fatal`、500 或 deadline/timeout；少量 `invalid salt: 0` 是探针新连接时的 MTProto 盐刷新噪声，未伴随业务 RPC 失败。通话媒体 relay/SCTP/RTMP、外部短信/邮件 provider、支付结算、真实 Passkey RP/认证器、第二 DC、原生频道完整媒体存储、礼物库存结算和私密聊天完整明文语义仍不计为生产完成。

## 2026-10-06 r22 Business Chat Links 生产双用户验收与 nil Peer 修复

- 发现并修复删除后解析未知 Business Chat Link 的编码崩溃：`account.resolveBusinessChatLink` 原先对不存在 slug 返回 `account.ResolvedBusinessChatLinks` 且 `Peer=nil`，Layer 229 编码器对 `Peer` 无条件编码，真实客户端会等待而不是收到结果。r22 现在对不存在或已删除 slug 返回 `INPUT_REQUEST_INVALID`；创建、编辑请求也拒绝空参数和未知 slug，不再隐式新增记录。新增组件回归覆盖随机 slug、跨用户解析、编辑、删除和删除后错误。
- 从当前源码构建并热替换 `backend-backend-1` 的 BFF 为 r22，SHA-256 `0dc00a499dd1b09c40ff7a73c6aefc426d50bb8d6b4a9aaa46ef5284418bbea2`，PID `1951`；r21 保留在容器中，BFF 日志级别为 `info`。只替换 BFF 进程，没有重启数据库、Redis、Kafka、etcd、session、msg 或其它服务。
- 真实双用户生产探针 `codex-link-1791270343677-94227` 通过 DC2 WebSocket -> gateway -> session -> r22 BFF -> APIFull/Redis-MySQL KV -> 客户端完成：创建返回 `BusinessChatLink`、owner `get` 返回 `account.BusinessChatLinks`、第二用户解析返回 `account.ResolvedBusinessChatLinks`、编辑返回更新后的 `BusinessChatLink`、再次解析读到新内容、删除返回 `boolTrue`/`Boolean(true)`，删除后解析收到预期 `INPUT_REQUEST_INVALID`。探针退出码为 0。
- 探针在生产 Redis 中保存两位用户 `blink:*` 的运行前字节值，结束后逐字节比较一致；tagged 临时链接为 0。生产 `blink:slug:*` 索引中属于用户 `136907713` 的残留为 0。没有执行 schema 迁移或清库。
- `go test -c -o /tmp/teamgram-apifull-core.test ./app/bff/apifull/internal/core`、Linux ARM64 BFF 构建和 `git diff --check` 通过。方法台账仍为 813 行、813 个唯一方法；本轮 4 个 Business Chat Link 方法提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。这组方法有生产真实链路证据，但完整 Layer 229 仍未达到 813/813。

## 2026-10-06 r23：生产文件上传与媒体读回

- 从当前工作树交叉编译 Linux ARM64 BFF，SHA-256 为 `984c307fde00a1b6e67918b9830cd2e5e1f19cae6c3ebc1e4562a6ccdba53474`；在 `backend-backend-1` 保留 r22 副本后只热替换 BFF 为 `/app/bin/bff-r23`。容器保持 `running`、`restart=0`、`oom=false`，DFS、media、biz、msg、authsession、session、数据库、Redis、Kafka、etcd 均未重启。
- 使用生产 MySQL 中已登记的永久 auth key `136907713`，通过 `127.0.0.1:11443` 完成真实 DC2 WebSocket → gateway → session → r23 BFF → DFS/media → 生产对象存储/数据库链路。`upload.saveFilePart` 和 `upload.saveBigFilePart` 均返回 `Boolean`；对应 `InputFile`/`InputFileBig` 被 `messages.uploadMedia` 消费，`messages.sendMedia` 返回 `Updates`，历史返回 `messages.Messages`，`upload.getFile` 返回 `upload.File`，下载字节与上传 payload 精确一致。
- 生产探针 tag 为 `prod-upload-1791274953030-20586`。临时自对话消息已调用 `messages.deleteMessages` 清理；没有留下可见探针消息。双用户全链路、可逆流程、多媒体相册和业务链接探针随后继续通过；最近日志没有新的 panic、fatal、500 或 deadline/timeout。
- `upload.saveFilePart`、`upload.saveBigFilePart`、`messages.uploadMedia`、`messages.sendMedia`、`upload.getFile` 的生产验收证据已写入 `LAYER229_METHOD_LEDGER.csv`。多片重启持久性、对象存储故障切换、CDN/hash provider、加密文件和其它媒体类型仍保持部分验收或实现阻塞，未被提升为完整 Layer 229 通过。
- 追加探针 `prod-upload-1791275264463-13018` 在同一生产链路对已上传文档调用 `upload.getFileHashes`，返回文件哈希向量；临时消息清理成功。该方法从 `NOT_ACCEPTED` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`，大文件分页、重启持久性与 CDN hash 仍未验收。

## 2026-10-06 r25/r15：Sync Kafka 修复与可重启生产镜像验收

- 修复 `app/messenger/sync/internal/server/mq/mq.go` 对 `sync.pushUpdatesIfNot` 和 `sync.pushBotUpdates` 的漏分发。Kafka 消费者现在识别两种完整 TL 消息、反序列化后调用对应 `SyncCore` handler；此前它们会落入 `invalid key` 默认分支并丢弃。新增 `mq_test.go` 覆盖两种消息的分派路径。`go test ./app/messenger/sync/internal/server/mq ./app/messenger/sync/internal/core -count=1` 通过。
- 收紧 `messages.editFactCheck`、`messages.deleteFactCheck`、`messages.getFactCheck` 的输入和消息归属校验：要求认证、合法 peer/消息 ID，并通过权威消息读取器确认消息存在且属于请求 peer；未知消息、错 peer、空请求和缺失读取器均 fail closed。现有 caller-scoped KV 行为保持不变，事实核查共享 provider、权限模型、canonical 持久化和 update 投递仍未实现。
- 从当前工作树交叉编译 Linux ARM64 二进制并核对摘要：BFF `/tmp/teamgram-bff-r25` SHA-256 `cccf268e97576ca2327e6707f80c38a1ed12051b44a79ecdd9b42da93c003093`；Sync `/tmp/teamgram-sync-r25` SHA-256 `a7a0dbdc0c39e30bf8af6820f05387812deaa4042b7422737fce3054bce91db8`。`go test -c -o /tmp/teamgram-apifull-core-r25.test ./app/bff/apifull/internal/core` 与 `git diff --check` 通过。
- 当前台账仍为 813 行、813 个唯一方法。结构化计数：生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=285`、`PARTIAL_ISOLATED_SESSION_E2E=258`、`PARTIAL_COMPONENT_ONLY=139`、`NOT_ACCEPTED=60`、`PARTIAL_ISOLATED_DB_ONLY=42`，另有隔离组件 10、隔离库 8、传输包装 11；这些状态表示验收证据级别，不把缺少权威 provider 的方法伪装成已实现。

## 2026-10-07：群通话邀请与订阅控制面生产复验

- 使用生产 MySQL 中登记的用户 `136907713` 永久 auth key，在 `backend-backend-1` 上通过 DC2 WebSocket → gateway → session → BFF → APIFull/MySQL 运行 `docker/production/probe-group-call-control.ts`。探针创建临时自有群通话，`phone.exportGroupCallInvite` 返回带 `invite_hash` 的 `phone.ExportedGroupCallInvite`，`phone.toggleGroupCallStartSubscription` 写入订阅后由 `phone.getGroupCall` 读回 `schedule_start_subscribed=true`，`phone.joinGroupCall` 返回 typed `Updates`，最后 `phone.discardGroupCall` 后再次读取得到 `GROUPCALL_INVALID`。
- 生产 MySQL 对本次 call、invite、subscription、participant 的行数均为 0；探针退出码为 0，没有留下业务数据。`phone.createGroupCall`、`phone.joinGroupCall`、`phone.discardGroupCall` 的生产证据已补入台账，邀请和订阅从 `PARTIAL_COMPONENT_ONLY` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。
- 当前 GramJS 运行时没有导出 `getGroupCallJoinAs`、录制、群通话消息等 Layer 229 构造器；`joinGroupCallPresentation` 的返回更新也无法由该客户端解码，因此没有把这些客户端构造器缺口误记为服务端完成。录制消费者、媒体 relay/RTMP、跨用户通知和通话 Stars 结算仍未接入。

## 2026-10-07：铃声保存回环生产复验

- 使用同一生产永久 auth key，`docker/production/probe-saved-ringtones-roundtrip.ts` 经 DC2 WebSocket → gateway → session → BFF → APIFull/MySQL 对 `account.saveRingtone` 和 `account.getSavedRingtones` 做了可逆回环。随机临时 `InputDocument` 保存返回 `account.SavedRingtone`，列表读回真实文档和 access hash，使用返回 hash 得到 `account.SavedRingtonesNotModified`，取消后列表不再包含该 ID。
- 探针结束后 MySQL `ring:136907713` 恢复为空，退出码为 0。由于当前 GramJS 运行时未导出这两个较新的构造器，探针使用原始 Layer 229 constructor 编码；全局铃声目录、上传 provider 和跨设备同步仍未验收。

## 2026-10-07：本轮台账收口

- 当前 CSV 重新结构化解析为 813 行、813 个唯一方法：生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=285`、`PARTIAL_ISOLATED_SESSION_E2E=254`、`PARTIAL_COMPONENT_ONLY=143`、`NOT_ACCEPTED=60`、`PARTIAL_ISOLATED_DB_ONLY=42`，另有隔离组件 10、隔离库 8、传输包装 11。生产级全量目标仍不是 813/813；短信/邮件 provider、支付/礼物结算、Passkey/QR 信任注册、多 DC、原生频道媒体、通话媒体控制面和秘密聊天客户端语义仍是主要阻塞。
- 本轮工作树检查：`git diff --check` 通过；`go test ./app/service/biz/user/internal/core -run TestSavedMusicEntryMatchesRequestedDocument -count=1` 通过；`backend-backend-1` 为 `teamgram-server-latest:20261007-r17-prod`，`running`、`restart=0`、`oom=false`；生产探针残留进程为 0。

## 2026-10-07：Terms of Service 只读生产链路

- `help.getTermsOfServiceUpdate` 的 handler 按当前用户读取共享 APIFull `tos:<user_id>` 接受标记；未接受时返回 `help.TermsOfServiceUpdate`，已接受时返回 `help.TermsOfServiceUpdateEmpty`，两者都带一小时过期时间。当前部署的条款内容仍是内置的 `{"country":""}` / `Terms of Service` 占位文本，没有可配置的条款 provider。
- 在 `backend-backend-1`（r16，容器 `running`、`restart=0`、`oom=false`）上，用生产 MySQL 中已登记的永久 auth key 通过真实 DC2 WebSocket → gateway → session → Tos BFF → APIFull/MySQL 调用该无参数 RPC。GramJS 运行时未导出该请求构造器，探针 `docker/production/probe-tos-read.mts` 使用原始 Layer 229 constructor `0x2ca51fd1` 编码；服务端返回 typed `help.TermsOfServiceUpdate`，包含 placeholder ID/text、`popup=false`、零实体和未来约一小时的 `expires`。
- 探针前后只读核对 `apifull_kv` 的 `tos:136907713`（均为空）及该用户 active `auth_users` 行数（均为 1），未调用 `help.acceptTermsOfService` 或其他写/破坏性 RPC。探针显式退出，工作树中旧的 `tsx` 生产探针也已停止，避免继续产生 `AUTH_KEY_UNREGISTERED` 日志。
- 因此该方法从路由/未验收提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。这只证明生产 typed 读取链路和当前静态占位分支；`help.acceptTermsOfService`、可配置条款版本、接受后的 empty 分支、跨重启持久性和完整 ToS 产品流程仍未通过。

## 2026-10-06 r15：联系人要求只读生产验收

- 使用 r15 生产容器中已登记的用户 `136907713` 永久 auth key 和用户 `136907714` 的生产 access hash，通过 `127.0.0.1:11443` 运行只读探针 `docker/production/probe-requirements-to-contact.ts`。真实链路为客户端 → DC2 WebSocket gateway → session → privacysettings BFF → User service → 生产 MySQL；`users.getRequirementsToContact` 返回 `Vector<RequirementToContact>`，目标用户当前无付费消息/高级联系人限制，向量元素为 `RequirementToContactEmpty`。探针退出码 0，没有写入或临时数据。
- 该方法从 `NOT_ACCEPTED` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`；真实付费 Stars / premium 分支、联系人豁免分支以及重启持久性仍未验收。当前生产级全量目标仍未完成。

## 2026-10-07：account.checkUsername 生产只读链路

- 使用生产 MySQL `auth_keys`/`auth_users` 中已登记的用户 `136907714` 永久 auth key，仅在单次本地探针进程环境变量中注入；没有打印、写入仓库或写入生产表。探针为 `docker/production/probe-account-check-username-readonly.ts`，会生成合法且高熵的候选用户名，不调用任何写入 RPC。
- 在 `backend-backend-1`（r16，容器保持 `running`、`restart=0`、`oom=false`）上，经 DC2 WebSocket → gateway → session → usernames BFF → User service/MySQL 调用 `account.checkUsername`。候选 `codexauditmux1gkwm7a5` 返回类型化 `Boolean`（底层值为 true），`writes=0`，探针退出码为 0，没有 400/500、超时或连接中断。
- 该项从 `PARTIAL_COMPONENT_ONLY` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。本次只证明可用用户名和真实 User provider 的读取路径；已占用用户名、本人已有用户名、重启持久性和 `account.updateUsername` 的写入/同步仍未在生产验收。
- 热替换验证通过后，确认仅热替换不具备容器重建持久性；据此新增 `docker/production/Dockerfile.backend-r15` 与 `build-r15.sh`，可从当前工作树重新构建 Linux ARM64 BFF/sync，再以生产 r14 镜像为基底封装；新增 Compose 覆盖文件保留只读 `bff.production.yaml` 挂载并以 `build: !reset null` 移除旧部署目录中的过时 build context。使用 `docker compose ... up -d --no-deps --no-build --force-recreate backend` 只重建 `backend-backend-1`，MySQL、Redis、Kafka、etcd、session 外部服务未重启。r15 镜像 ID 为 `sha256:203194df7bfb93555bfd6dcd2fc82352d9cfc674f1b69ca409eec30214606582`；原 r14 镜像保留作回滚目标，不执行数据库迁移或清库。
- r15 启动后 11 个 Teamgram 进程均由 `/app/docker/entrypoint.sh` 标准脚本启动；容器 `running`、`restart=0`、`oom=false`，生产配置 bind mount 保持只读，`/app/bin/bff` 与 `/app/bin/sync` 的 SHA-256 分别为 `cccf268e97576ca2327e6707f80c38a1ed12051b44a79ecdd9b42da93c003093` 和 `a7a0dbdc0c39e30bf8af6820f05387812deaa4042b7422737fce3054bce91db8`。etcd endpoint healthy；生产 Kafka 消费者处理双用户探针更新，未出现 `invalid key`。
- 在 r15 容器重建之后再次使用生产 MySQL 中登记的用户 `136907713` / `136907714` 永久 auth key（只注入单次本地探针进程）执行 `probe-real-chain.ts`。DC2 WebSocket → gateway → session → r15 BFF → msg/biz/APIFull 返回预期的认证、配置、对话、私聊搜索/全局搜索/已读/编辑、反应、投票、论坛群、定时消息及 canonical 频道成员/详情/消息/搜索/已读/阅读数/差分/编辑/删除类型；频道阅读数为 `messages.MessageViews`，差分为 `updates.ChannelDifference`，删除后按 ID 得到 `MESSAGE_ID_INVALID`。最终探针 tag 为 `prod-chain-1791285844112-39271`，退出码 0；生产 MySQL 精确核对 active 探针 `messages=0`、`apifull_group_call=0`、未删除临时群 `chats=0`。无 500 或 deadline/timeout；日志中的 `invalid salt: 0` 是新连接盐刷新，`MESSAGE_ID_INVALID` 是探针对已删除对象的预期断言。
- 当前台账为 813 行、813 个唯一方法。生产列结构化计数：`BLOCKED_BY_IMPLEMENTATION_GAP=286`、`PARTIAL_ISOLATED_SESSION_E2E=230`、`PARTIAL_COMPONENT_ONLY=160`、`NOT_ACCEPTED=66`、`PARTIAL_ISOLATED_DB_ONLY=42`，另有隔离组件 10、隔离库 8、传输包装 11。短信/邮件 provider、支付/礼物结算、真实 Passkey RP/认证器、第二 DC、原生频道完整媒体/CDN、通话媒体控制面和秘密聊天客户端明文语义仍有阻塞；完整生产级 Layer 229 不是 813/813。
- 当前台账仍为 813 行、813 个唯一方法。结构化计数：生产列 `BLOCKED_BY_IMPLEMENTATION_GAP=286`、`PARTIAL_ISOLATED_SESSION_E2E=230`、`PARTIAL_COMPONENT_ONLY=160`、`NOT_ACCEPTED=66`、`PARTIAL_ISOLATED_DB_ONLY=42`，另有隔离组件 10、隔离库 8、传输包装 11；这些状态表示验收证据级别，不把缺少权威 provider 的方法伪装成已实现。

## 2026-10-07：群通话与铃声回环补充

- `docker/production/probe-group-call-control.ts` 在 `backend-backend-1` 上用生产用户 `136907713` 的有效永久 auth key 完成临时群通话创建、`phone.exportGroupCallInvite`、`phone.toggleGroupCallStartSubscription`、`phone.getGroupCall` 订阅读回、`phone.joinGroupCall` 和销毁后的 `GROUPCALL_INVALID` 校验。MySQL 对 call、invite、subscription、participant 的探针行均为 0；邀请和订阅的生产证据已写入台账。
- `docker/production/probe-saved-ringtones-roundtrip.ts` 通过原始 Layer 229 构造器完成 `account.saveRingtone` 保存/取消、`account.getSavedRingtones` 文档读回和 hash-not-modified；`ring:136907713` 清理后为空。全局铃声目录、上传 provider、通话媒体消费者和当前 GramJS 未导出的较新构造器仍未验收。
- 当前 CSV 结构化计数为 `BLOCKED_BY_IMPLEMENTATION_GAP=285`、`PARTIAL_ISOLATED_SESSION_E2E=258`、`PARTIAL_COMPONENT_ONLY=139`、`NOT_ACCEPTED=60`、`PARTIAL_ISOLATED_DB_ONLY=42`，另有隔离组件 10、隔离库 8、传输包装 11；完整 Layer 229 生产级验收仍不是 813/813。

## 2026-10-07：群通话消息返回编码与删除生产复验

- 修复 `app/bff/apifull/internal/core/group_calls.go` 的群通话消息 update 构造：Layer 228 选择旧版 `updateGroupCallMessage#d8326f0d` 时，之前只填了新版字段，session 在编码空的 `GroupCallMessage` 时触发 nil pointer，客户端表现为超时。现在同时填充旧版必填的消息 ID、发送者、日期和 `TextWithEntities`，并保留新版字段。
- 从当前工作树构建 Linux ARM64 BFF（SHA-256 `ef8aaef98156eeb007b514bd1c7c75611e0f407fdc3ba6c6bba5bbdf281d6e45`），在 `backend-backend-1` 中保留旧 `/app/bin/bff` 为 `bff.before-group-call-20261007`，只热替换 BFF 进程为 PID 252。容器仍为 `teamgram-server-latest:20261007-r17-prod`、`running`、`restart=0`、`oom=false`；数据库、Redis、Kafka、etcd、session、msg 和其它服务没有重启。
- 使用生产用户 `136907713` 的永久 auth key，经 DC2 WebSocket -> gateway -> session -> BFF -> APIFull/MySQL 执行 `docker/production/probe-group-call-settings.ts`，临时 call `1791318239046456007` 完成设置读回、录制控制、参与者编辑、`phone.sendGroupCallMessage`、`phone.deleteGroupCallMessages`、`phone.deleteGroupCallParticipantMessages`、录制停止和销毁。发送、删除和参与者删除均返回 typed `Updates`；消息 JSON/text 和 random_id 从生产 MySQL 读回，删除先将消息行标为 `deleted=1`，销毁后 call、settings、participant、message 行均为 0。探针退出码为 0。
- 生产日志窗口未出现新的 nil pointer、panic、500 或业务超时；失败探针留下的临时数据也已由 finally 清理。`phone.sendGroupCallMessage`、`phone.deleteGroupCallMessages`、`phone.deleteGroupCallParticipantMessages` 从 `PARTIAL_COMPONENT_ONLY` 提升为 `PARTIAL_ISOLATED_SESSION_E2E`。跨用户消息广播、媒体 relay、Stars 结算和重启持久性仍未验收。
- CSV 结构化校验确认仍为 813 行、813 个唯一方法、8 列；生产列当前为 `BLOCKED_BY_IMPLEMENTATION_GAP=285`、`PARTIAL_ISOLATED_SESSION_E2E=267`、`PARTIAL_COMPONENT_ONLY=133`、`NOT_ACCEPTED=60`、`PARTIAL_ISOLATED_DB_ONLY=39`，另有隔离组件 10、隔离库 8、传输包装 11。完整 Layer 229 生产级验收仍不是 813/813。

## 2026-10-07：Terms of Service 接受回环生产复验

- 新增 `docker/production/probe-tos-roundtrip.ts`，使用生产 MySQL 中登记的用户 `136907714` 永久 auth key，经 DC2 WebSocket -> gateway -> session -> Tos BFF -> APIFull/MySQL 调用 `help.acceptTermsOfService`。
- RPC 返回成功 Boolean，`tos:136907714` 从空值写为当前条款 ID `{"country":""}`；随后调用 `help.getTermsOfServiceUpdate` 返回 typed `help.TermsOfServiceUpdateEmpty`，证明接受状态被服务端读取并编码。
- 探针 finally 将原本的空值行恢复，并再次核对存在性和值完全一致；没有永久生产数据、容器重启或其他服务变更。该方法从 `ROUTED_NOT_FUNCTIONALLY_ACCEPTED` / `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。
- 本次只验收当前内置占位条款的接受、读回和可逆恢复；可配置条款 provider、版本历史、跨重启持久性及真实非空条款内容仍未验收。

## 2026-10-07：频道主资料页签回环生产复验

- 新增 `docker/production/probe-channel-profile-tab-roundtrip.ts`，使用生产用户 `136907714` 的永久 auth key 和其拥有的真实频道 `1`，经 DC2 WebSocket -> gateway -> session -> UserChannelProfiles BFF -> APIFull/MySQL 调用 `channels.setMainProfileTab`。
- handler 的频道管理员校验通过，RPC 返回 BooleanTrue，`profile-tab:1` 写入 `profileTabPosts` 的持久化 JSON。探针 finally 删除原本不存在的 KV，并再次核对不存在，未留下生产数据或重启服务。
- 该方法从 `ROUTED_NOT_FUNCTIONALLY_ACCEPTED` / `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。跨会话通知、重启持久性和客户端其它页签消费仍未验收。

## 2026-10-07：参与者反应删除双用户生产回环

- 新增 `docker/production/probe-reaction-participant-delete.ts`，从生产 MySQL 读取已登记用户 `136907713` / `136907714` 的永久 auth key 和 access hash，经 DC2 WebSocket -> gateway -> session -> APIFull/Redis 建立真实私聊并发送两条临时消息。
- 用户 B 对两条消息分别发送 `👍` / `🔥`，读回的参与者反应计数均为 1。`messages.deleteParticipantReaction` 返回 typed `Updates` 并使第一条计数变为 0；再次写入第二条反应后，`messages.deleteParticipantReactions` 返回 BooleanTrue 并使第二条计数变为 0。
- finally 删除临时消息并恢复共享反应、个人反应列表、最近反应及操作记录 Redis 键的原始存在性和值。两个方法从 `ROUTED_NOT_FUNCTIONALLY_ACCEPTED` / `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`；频道管理员语义、跨设备通知和审核工作流仍未验收。

## 2026-10-07：个人频道绑定回环生产复验

- 新增 `docker/production/probe-personal-channel-roundtrip.ts`，使用用户 `136907714` 的生产永久 auth key 和其拥有的频道 `1`，经 DC2 WebSocket -> gateway -> session -> UserChannelProfiles BFF -> User service/MySQL 调用 `account.updatePersonalChannel`。
- 生产 RPC 返回 Boolean，`users.personal_channel_id` 从 `0` 改为 `1`；随后 `users.getFullUser` 返回 typed `users.UserFull`。当前 users BFF 没有 `PersonalChannelPlugin`，所以该响应不暴露 optional `personalChannelId` 字段，但权威 DB 字段已读回。
- 探针 finally 恢复原始 DB 值和 `user_data.2#136907714` Redis 缓存，并逐字节核对一致，没有永久生产数据或服务重启。该方法从 `ROUTED_NOT_FUNCTIONALLY_ACCEPTED` / `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`；RPC 自身的频道所有权校验、跨会话通知和重启持久性仍未验收。

## 2026-10-07：快捷回复可用性与排序生产回环

- 新增 `docker/production/probe-quick-replies.ts`，使用生产用户 `136907713` 的永久 auth key，经 DC2 WebSocket -> gateway -> session -> BFF -> APIFull/MySQL 完成 `messages.checkQuickReplyShortcut`、`messages.editQuickReplyShortcut`、`messages.getQuickReplies`、`messages.reorderQuickReplies` 和 `messages.deleteQuickReplyShortcut` 的可逆流程。
- 两个唯一临时快捷回复先被报告可用，创建后同名检查返回 `BoolFalse`；排序返回 typed `BoolTrue`，读回顺序与请求一致；删除后原顺序恢复。`qreply:136907713` 和 `qreply:msg:136907713` 在清理后均不存在，探针退出码为 0。
- 当前 GramJS runtime schema 没有导出部分新构造器，探针对 Layer 229 使用原始 constructor 编码；服务端路由实际进入 `RPCBusinessQuickReply`，无超时或 5xx。上述两个方法已从 `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。跨设备通知、重启复验和外部快捷回复 provider 仍未验收。

## 2026-10-07：Promo 隐藏状态生产回环

- 新增 `docker/production/probe-promo-data-roundtrip.ts`，使用生产用户 `136907714` 经 DC2 WebSocket -> gateway -> session -> APIFull/MySQL 调用 `help.getPromoData`、`help.hidePromoData`、`help.getPromoData`。初始返回 `help.PromoData` peer `777000`，隐藏返回 typed Boolean true，后续返回下一个目录项 peer `777001`。
- 隐藏列表实际写入 `apifull_kv` 的 `promo:hide:136907714`，不是 Redis；探针删除该临时行并核对 MySQL 与 Redis 均恢复为探针前状态。退出码为 0，没有容器重启、超时或 5xx。
- 因此 `help.hidePromoData` 已从 `NOT_ACCEPTED` 提升为 `PARTIAL_SESSION_VERIFIED` / `PARTIAL_ISOLATED_SESSION_E2E`。当前 promo 目录仍是内置静态目录，跨设备同步和权威广告/推广 provider 尚未接入。
- 本轮收口后的 CSV 结构化计数为：`BLOCKED_BY_IMPLEMENTATION_GAP=286`、`PARTIAL_ISOLATED_SESSION_E2E=277`、`PARTIAL_COMPONENT_ONLY=131`、`NOT_ACCEPTED=52`、`PARTIAL_ISOLATED_DB_ONLY=39`，另有隔离组件 9、隔离库 8、传输包装 11；完整 Layer 229 生产级验收仍不是 813/813。

## 2026-10-07：支付 provider 响应认证与频道搜索游标边界

- 支付 provider 的 `get_form`、`validate_requested_info` 和结算响应现在都必须带 `X-Teamgram-Payment-Signature`，值为响应原始字节的 HMAC-SHA256 十六进制摘要。签名密钥由顶层 BFF 配置传入 APIFull；缺密钥、缺签名或签名不匹配时均拒绝处理。现有请求幂等键、MySQL payment ledger 和“仅 verified 后结算”逻辑保留。
- 频道搜索修正了 `offset_rate` 到 provider 参数的映射，并将普通历史读取上限与搜索上限分别限制在 50 和 100。该 cursor 目前只是单频道查询结果的 SQL 行偏移，仍缺全局帖子索引、paid-Stars provider、完整 Layer 229 排名语义和生产 session 验收。
- 此变更只加固通用 provider 契约；没有配置真实支付 provider，也没有实现 App Store/Play 票据核验、权益发放、退款回调或 Stars 结算。现有 provider 测试夹具已调整为签名响应，本轮未执行测试；`git diff --check` 通过。

## 2026-10-07：TURN 配置 fail-closed

- `phone.getCallConfig` 不再在缺少 relay 时返回 `127.0.0.1` 和内置 `teamgram/teamgram` 凭据。TURN 主机、有效端口、用户名和密码必须由 BFF 配置提供；loopback 与 `localhost` 会被拒绝。缺少任一配置时返回 `METHOD_NOT_IMPL`。
- 当前 `teamgramd/etc/bff.yaml` 只配置 `TurnHost` / `TurnPort`，没有真实 TURN 凭据，因此新代码部署后通话配置会 fail-closed。此前 2026-10-06 探针只证明 `DataJSON` 可编码，没有完成 TURN 连通或媒体协商；对应 ledger 状态已改回阻塞。
- `bots.getBotInfo` 现在从权威 User 服务读取 bot registry 描述与用户资料；APIFull 校验 `InputUser` access hash，普通用户不能通过输入他人 ID 读取 bot 资料。当前 User RPC 没有语言参数化资料，且尚无生产 session 复验；bot 创建、token、commands 管理和 webhook 仍未形成完整 registry/workflow。

## 2026-10-07：原生频道消息作者 provider

- `channels.getMessageAuthor` 按原生频道消息 ID读取作者：先校验 `channels` 的 access hash 和 `channel_participants` 成员状态，再从 `channel_messages.message_data.from_id` 解析用户作者；频道身份发布返回空作者。APIFull 自有频道继续使用其 canonical 频道消息存储。legacy 频道错 hash 或已删除时直接拒绝，不回退到同 ID 的 APIFull 记录。
- 新增 APIFull/原生频道、成员、匿名发布、错 hash 和同 ID 存储碰撞的隔离 DB 回归覆盖，以及只读生产探针。当前只完成 `channelview` 和 `domain` 包编译；未运行测试或生产探针，因此台账仍为部分组件验收。

## 2026-10-07：Bot 命令 registry provider

- `bots.setBotCommands`、`bots.resetBotCommands` 和 `bots.getBotCommands` 改为读写 User 服务的 canonical `bot_commands` registry，不再使用 APIFull Redis 命令副本。User 服务保留 creator 权限，并允许 bot 身份只修改自己的命令；替换仍在同一数据库事务内完成。
- 当前 provider 仅支持默认 scope 和空语言码，其他 scope/翻译 fail closed；命令读取错误也会原样返回。APIFull 与 User core 编译通过，现有 provider fixture 已更新但未运行；数据库、session 和生产路径未验收，创建 bot、token 和管理工具仍未接通。

## 2026-10-07：支付 settlement provider 幂等

- `payments.sendPaymentForm` 现在把用户 ID 和本地 payment request key 做 SHA-256，作为稳定的 `Idempotency-Key` 发送给结算 bridge。并发 RPC 重试可以用同一键在外部扣款前去重；bridge 必须将该键传递给或映射到支付处理器的幂等机制。
- 本地 MySQL request/receipt ledger 和 HMAC 响应校验仍是结算依据。未配置外部 provider，也未运行测试、session 或生产结算；因此此项仍为 `BLOCKED_BY_IMPLEMENTATION_GAP`，不能据此发放权益或记作支付验收。

## 2026-10-07：认证 provider 传输重试去重

- 每次验证码投递生成独立随机 `DeliveryID`，通用 HTTP SMS/Email provider 在 payload 和 `Idempotency-Key` 请求头中发送该值。一次投递内的网络/5xx 重试共享 ID；不同验证码投递不会复用可能固定的 challenge ID。
- 此处只为 provider 提供去重键；外部 provider 必须实际执行幂等去重。没有配置真实短信/邮件端点，也没有执行测试或生产验收，`auth.sendCode` 仍是部分组件验收。

## 2026-10-07：TURN 按用户签发短期凭据

- `phone.getCallConfig` 可使用 coturn REST shared secret 为当前用户签发 `expiry:user_id` 用户名和 HMAC-SHA1/Base64 密码；默认 TTL 为一小时，最大 24 小时。现有静态凭据保留兼容；TURN host、端口或凭据缺失时仍 fail closed。
- shared secret 已从顶层 BFF 配置传递到 APIFull domain，不写入返回值或日志。当前配置未提供 shared secret 或静态凭据，也没有真实 TURN 连通、WebRTC 协商或生产验收；群直播 stream channels/RTMP 仍需媒体控制面。

## 2026-10-07：Bot 用户名查询接入全局 registry

- `bots.checkUsername` 现在校验标准用户名格式与 `bot` 后缀，再调用 User 服务的 `UsernameDAO` 检查全局占用；User 服务不再吞掉 DAO 错误，因此查询故障会作为错误返回。
- 查询仅用于可用性提示，不能锁定名字；`bots.createBot` 已实现用户名事务预留、bot User/registry 创建和 token 发行，并校验父 Bot 的管理能力。此处尚未应用新 schema migration 或运行真实创建回环，因此仍没有生产验收。

## 2026-10-07 r19：全服务镜像与双用户生产链路续验

- 全仓 `go build ./...` 通过。构建期间修正了 bot-token User RPC 的 `mtproto.String` 字段访问（使用生成协议字段 `V` / `GetV`），User 服务和 BFF 均可编译。
- 新增 r19 ARM64 镜像构建文件，从 r18 运行时层保留现有配置和入口，只用当前源码重建 idgen、status、dfs、media、authsession、biz、msg、sync、bff、session、gnetway 共 11 个服务二进制。部署到目标 `backend-backend-1` 后，镜像为 `teamgram-server-latest:20261007-r19-prod`，11 个进程均运行，backend `restart=0`、`oom=false`。
- 部署前后生产 MySQL schema 指纹均为 `0cda7004c0acfb1e8dcc4942cadc58be3bafeca8ed6fdbb5058936584c2df690`；未执行 SQL migration，MySQL、Redis、etcd 容器 ID 和重启计数未变。Kafka 在 backend 部署前有一次独立重启，随后 broker API 和 topic 列表探测通过；本轮没有重启 Kafka。
- 生产用户 `136907713` 与 `136907714` 的现有永久授权经真实 DC2 WebSocket -> gateway -> session -> r19 全部服务 -> 生产 MySQL 完成双用户探针，tag `prod-chain-1791340521200-31219`。配置/目录/对话、私聊搜索与已读/编辑、反应、双用户投票与未读/已读、论坛群、定时消息，以及原生频道成员/详情/消息/搜索/已读/阅读数/差分/编辑/删除均返回预期 typed constructor。
- 修正探针清理后重新执行：未读投票由投票者 B 自己读回并标记已读；定时消息 finally 只删除仍待发送 ID。生产库回读确认该 tag 的活跃消息为 0、活跃群为 0、频道/APIFull 频道记录为 0、tagged APIFull KV 为 0；只保留预期的停用群 tombstone。此前一轮探针留下的两条 tag JSON 数组项已逐项移除，原有 KV 数组仍保留且长度各仅减少 1。
- 在 r19 上再次运行 `channels.getMessageAuthor` 只读生产探针：用户 `136907714` 对频道 `1` 的原生消息 ID `1` 收到 `User(id=136907714)`；错误使用用户消息盒 ID `5` 收到 `UserEmpty`；错误 access hash 收到预期 `CHANNEL_INVALID`。消息和频道行前后 SHA-256 相同，writes=0。handler focused tests 通过；隔离数据库授权用例仍未运行，因为 `APIFULL_MYSQL_DSN` 未配置且测试端口不可达。
- 本轮窗口内没有 panic、5xx 或 deadline/timeout；日志中的 `MESSAGE_ID_INVALID` 和 `CHANNEL_INVALID` 是探针对已删除消息及错误 access hash 的预期 400 断言。台账仍有 813 个方法记录，r19 后 `PARTIAL_ISOLATED_SESSION_E2E=278`、`BLOCKED_BY_IMPLEMENTATION_GAP=284`、`PARTIAL_COMPONENT_ONLY=133`，完整 Layer 229 生产级验收尚未完成。

## 2026-10-07 r19：只读配置与保存对话复验

- 使用生产 MySQL 中用户 `136907714` 的现有永久授权，经 DC2 WebSocket -> gateway -> session -> r19 BFF 只读复验 9 个 Layer 229 方法。配置返回 `help.AppConfig(hash=0, value=0)`，用户信息为 `help.UserInfo(entities=0)`，peer 与 profile palette 各返回 7 个颜色；附件菜单 bot、置顶保存对话为空。
- 保存对话方法命中真实数据：`messages.getSavedDialogs` 返回 1 个对话、1 条消息和 1 个用户；`messages.getSavedHistory(InputPeerSelf)` 返回 4 条消息；`messages.getSavedDialogsByID(ids=[InputPeerSelf])` 返回对应的 1 个对话、1 条消息和 1 个用户。`getSavedDialogsByID` 原先的探针错误地传空 ID 向量，已修正为真实 `InputPeerSelf` 后重新调用。
- 结果更新在 `LAYER229_METHOD_LEDGER.csv` 对应记录中。上述方法仍是部分验收：动态配置、非空 TSF、attach-menu bot registry、置顶保存对话、原生频道 hydration 与重启持久性没有由本次探针覆盖；全量 813 方法验收仍未完成。

## 2026-10-07 r19：媒体搜索生产读回

- `docker/production/probe-search-media-readonly.ts` 使用生产用户 `136907714` 和已存在的频道 3，通过 DC2 WebSocket -> gateway -> session -> BFF 查询照片计数、搜索位置和已发送媒体，并与生产 MySQL 行数核对。`messages.getSearchCounters` 返回照片 `count=1`，与频道媒体记录一致；`messages.searchSentMedia` 返回消息 ID `3`，与发送照片记录数 `1` 一致。
- `messages.getSearchResultsPositions` 返回 `SearchResultsPositions(count=0, positions=0)`，与本次 `InputPeerSelf` 照片查询对应的 MySQL 计数 `0` 一致。本次只验收其空结果分支，不能证明非空全局排名、频道或 saved-peer 排名。
- `messages.getSearchResultsCalendar` 在 7 个真实用户/群对话上返回与 `messages.search` 总数一致的合法空媒体日历，period 和 representative vectors 均为零；非空分桶仍未覆盖。
- 四项结果已记入方法台账；没有执行生产写操作。当前 CSV 生产列为 `PARTIAL_ISOLATED_SESSION_E2E=283`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=130`，仍有 813 项总量且未完成全量生产验收。

## 2026-10-07 r19：共同群只读生产读回

- 对生产用户 `136907714` 查询用户 `136907713` 的共同群。`messages.getCommonChats` 经 DC2 WebSocket -> gateway -> session -> Chats BFF -> chat service/MySQL 返回 `messages.Chats(chats=2)`；生产 MySQL 按 chat 服务使用的活跃成员条件核对到 2 个共同基础群，APIFull 共同频道集合为 0。
- 探针只读取用户与成员关系，未执行写操作。该结果覆盖真实共同基础群及实体 hydration；其它用户对、分页，以及 APIFull 中非空的共同频道仍未验收。方法证据已更新在 `LAYER229_METHOD_LEDGER.csv`。

## 2026-10-07 r19：账户外观只读复验

- 生产用户 `136907714` 经 DC2 WebSocket -> gateway -> session -> BFF 读取 `account.getWallPapers`、`account.getThemes`、`account.getChatThemes` 和 `account.getContentSettings`。壁纸/主题/聊天主题均返回合法空集合；三种 hash 回读分别返回对应的 NotModified 构造器，`wallpaper:136907714` 与 `theme:136907714` 的 KV SHA-256 前后不变。
- `account.getThemes` 与 `account.getChatThemes` 已从组件级提升为部分生产 session 验收；现有证据只覆盖空目录和 hash 分支，不覆盖上传文件形成的非空主题，也不代表接入全局壁纸/主题目录。`account.getContentSettings` 本次只复验了响应构造器。
- 当前 CSV 生产列为 `PARTIAL_ISOLATED_SESSION_E2E=285`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=128`。813 项全量验收仍未完成。

## 2026-10-07 r19：Bot 用户名可用性查询

- 生产用户 `136907714` 通过真实 DC2 WebSocket -> gateway -> session -> APIFull -> User 服务检查了合法且在生产索引中不存在的 bot username。由于客户端 GramJS 运行时缺少声明中对应的请求类，探针按 Layer 229 constructor `0x87f2219b` 编码；服务返回 Boolean `true`，MySQL `username` 行在调用前后均为 0。
- `bots.checkUsername` 已记为部分 session 验收。该查询本身是 advisory；占用名分支及 `bots.createBot` 的事务/竞态路径虽已实现，仍需独立 session 验收。当前 CSV 生产列为 `PARTIAL_ISOLATED_SESSION_E2E=286`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=128`、`NOT_ACCEPTED=50`。

## 2026-10-07 r19：频道用户名可用性查询

- 用户 `136907714` 通过真实 DC2 WebSocket -> gateway -> session -> Usernames BFF -> User 服务检查自己拥有的频道 3，使用生产 MySQL 中的有效 access hash 查询合法未占用候选名。Layer 229 raw 请求 `channels.checkUsername#10e6bd2c` 返回 Boolean `true`，生产 `username` 表查询后仍为 0 行。
- `channels.checkUsername` 已记为部分 session 验收。此次只覆盖频道 owner、可用用户名路径；已占用/本频道已有名分支、非 owner 权限边界和后续 `channels.updateUsername` 写入/回读未覆盖。当前 CSV 生产列为 `PARTIAL_ISOLATED_SESSION_E2E=287`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=127`、`NOT_ACCEPTED=50`。

## 2026-10-07 r19：近期 Emoji 状态读取

- 生产用户 `136907714` 经真实 DC2 WebSocket -> gateway -> session -> APIFull 读取 `account.getRecentEmojiStatuses`，收到 `account.EmojiStatuses(hash=1, statuses=0)`；携带返回 hash 再读，收到 `account.EmojiStatusesNotModified`。`emojistatus:recent:136907714` 与 `emojistatus:136907714` 的 MySQL KV 摘要前后相同。
- 本次验证合法空历史、hash 分支和只读性，不证明非空 emoji status 历史或跨设备同步。当前 CSV 生产列为 `PARTIAL_ISOLATED_SESSION_E2E=288`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=126`、`NOT_ACCEPTED=50`；813 项全量验收仍未完成。

## 2026-10-07 r19：持久群通话只读回读

- 新增 `docker/production/probe-group-call-readonly.ts`，使用生产用户 `136907714` 的永久授权，经 DC2 WebSocket -> gateway -> session -> BFF -> MySQL 调用 `phone.getGroupCall`、`phone.getGroupParticipants` 与 `phone.checkGroupCall`。从其已有 `apifull_group_call` 记录读取 call ID/access hash；前两种响应类型正确，ID/hash 一致，参与者数 `1`、roster count/version `1` 均与持久化 JSON 数组长度一致，且结果包含本人 self participant；source 检查请求 2 个 source，准确只返回存在的调用者 source。
- 探针还比较了调用前后的 `apifull_group_call.participants` SHA-256，摘要相同，确认通话行未被修改。没有加入/离开、修改设置或发送媒体信令，writes=0。该证据加强三个方法的生产 session 验收；多页/过滤、远端通知、媒体协商和跨客户端状态投递仍未验收。

## 2026-10-07 r19：频道消息按 ID 生产读回

- 新增 `docker/production/probe-channel-messages-readonly.ts`，使用生产用户 `136907714` 的现有永久授权，经 DC2 WebSocket -> gateway -> session -> BFF -> APIFull/MySQL 调用 `channels.getMessages`，读取其拥有的频道 `1790115796396772258` 中已保存的消息 ID `7`。
- 服务返回 `messages.Messages`。频道 ID、消息 ID、发送者、时间、频道实体和正文 SHA-256 均与 MySQL 对应行一致；调用前后的消息行元组一致，writes=0。错 access hash 返回 `CHANNEL_INVALID`，已由另一生产授权登录但不在 MySQL roster 中的用户 `136907713` 收到 `USER_NOT_PARTICIPANT`。此项覆盖单条 APIFull 频道消息读取与权限边界，不代表 native channel provider、多 ID 顺序或重启持久性已验收。
- 同一只读探针还调用 `messages.getHistory(InputPeerChannel)`：生产返回 7 条 `messages.Messages`，最新优先顺序、ID、发送者、日期、正文 SHA-256 和频道实体均与 canonical MySQL 页逐条匹配。错 access hash 与非成员也被拒绝；7 条消息的查询快照前后相同。该证据覆盖 APIFull 频道历史首屏，不覆盖 native channel history 或后续页。

## 2026-10-07 r19：非活跃频道空结果生产复验

- 新增 `docker/production/probe-inactive-channels-readonly.ts`，使用生产用户 `136907714` 的现有永久授权，经 DC2 WebSocket -> gateway -> session -> BFF -> MySQL 调用 `channels.getInactiveChannels`。返回 `messages.InactiveChats`，`chats=0`、`dates=0`、`users=0`。
- 探针按相同 30 天 cutoff 读取生产 MySQL canonical `apifull_channel` / `apifull_channel_message`：该用户有 6 个 APIFull 频道，符合非活跃条件的频道为 0，与 RPC 返回逐项一致；探针只读，writes=0。此证据只覆盖真实空结果，不覆盖非空非活跃频道编码或清理流程。
- 对应台账提升为 `PARTIAL_ISOLATED_SESSION_E2E`。当前 CSV 共 813 项，`PARTIAL_ISOLATED_SESSION_E2E=289`、`BLOCKED_BY_IMPLEMENTATION_GAP=283`、`PARTIAL_COMPONENT_ONLY=125`、`PARTIAL_ISOLATED_DB_ONLY=38`、`NOT_ACCEPTED=50`、隔离组件 9、隔离库 8、传输包装 11；仍不能视为全量生产验收。
- 后续检查相邻方法 `channels.getAdminLog` 时确认生产 `apifull_channel_admin_log` 当前总计 0 行，用户 `136907714` 的 6 个频道也都没有事件；没有执行生产成员变更来制造日志，因此该方法继续保持组件级状态，等待真实事件数据验收。

## 2026-10-07 r20：contacts.search 只读生产会话验收

- 确认 `backend-backend-1` 使用 r20 镜像且运行中；该容器没有配置 Docker healthcheck，清除代理变量后 DC2 WebSocket `/apiws` Upgrade 成功。探针只从生产 MySQL 在进程内读取用户 `136907714` 的授权密钥，并使用 GramJS 调用 `contacts.search`。
- 生产 MySQL 样本用户 `777000` 与该账号无 `user_contacts` 关系。API 返回 `contacts.Found`，在 `results` 中返回一个 `PeerUser` 并 hydration 为一个 `User`；`results=1`、`users=1`、`my_results=0`、`chats=0` 与 MySQL 样本及关系一致。探针没有业务写入、部署或重启容器，密钥未输出或落盘。
- 此次只覆盖非联系人用户搜索与实体 hydration；`my_results` 联系人分类、频道/基础群结果及更广的查询边界仍未验收。证据已记录在 `LAYER229_METHOD_LEDGER.csv` 的 `contacts.search` 行。
## 2026-10-07 r21：联系人 hash 合同修复与生产验收

- 源码检查发现 `contacts.getContacts` 忽略了 Layer 229 的请求 hash，匹配时仍执行用户 hydration 并返回完整 `contacts.Contacts`。现按排序后的活跃联系人 ID 计算与 `contacts.getContactIDs` 相同的 hash；非零 hash 命中时返回 `contacts.ContactsNotModified`，hash=0 仍强制完整响应。
- `go test ./app/bff/contacts/internal/core` 与 `git diff --check` 通过。r21 ARM64 镜像从当前工作树构建，11 个 backend 服务二进制编译完成；部署使用运行容器记录的 Compose 主文件、override 与 r21 覆盖，只执行 `up -d --no-deps --no-build --force-recreate backend`，没有运行迁移。
- r21 生产只读探针使用用户 `136907714` 的既有授权，经 DC2 WebSocket -> gateway -> session -> Contacts BFF -> User 服务 -> MySQL 验证 `contacts.getContactIDs`、`contacts.getStatuses` 和 `contacts.getContacts`。ID 向量与 `user_contacts` 一致；联系人状态与 `user_presences` 一致；`getContacts(hash=0)` 返回 1 个 contact 和 1 个 User，当前非零 hash 返回 `contacts.ContactsNotModified`。调用前后数据快照相同，业务写入为 0。
- backend 镜像为 `teamgram-server-latest:20261007-r21-prod`，容器 `running`、`restart=0`、没有 Docker healthcheck；11 个服务进程存在。MySQL、Redis、etcd 容器 ID 和重启计数不变，MySQL schema 指纹前后均为 `255ad0524d0c70fdd0b97403c40ebfa034b9ecc5a77556fcd720eeaf4449817a`。Kafka 容器 ID 未变，但重启计数在镜像构建期间由 1 增为 2；它在 13:28（部署 backend 前）从非干净关闭恢复 56 个日志段，随后报告 `Kafka Server started`。Docker 事件缓存没有对应事件，确切根因未确认；未手动重启 Kafka。Kafka 内部 DNS `kafka:9092` 与宿主监听 `localhost:9094` 的只读 topic 查询均成功，返回 `Inbox-T`、`Sync-T` 和 `__consumer_offsets`；此前对 `localhost:9092` 的检查用了错误端口，不作为 Kafka 未就绪证据。
- 双账号 username 回环使用 `docker/production/probe-channel-username-resolution.ts`：用户 `136907714` 创建临时 APIFull 频道并分配随机 username，非成员用户 `136907713` 经真实 session 调用 `contacts.resolveUsername`，收到了匹配 PeerChannel 和非 creator Channel。创建者之后通过 RPC 清除 username 并删除频道；MySQL 回读确认对应频道行和活跃 username 索引均为 0。`channels.createChannel`、`channels.updateUsername` 与 `contacts.resolveUsername` 的证据已更新到台账。
- 只读核对还发现生产 username 表存在 2 条活跃的 `PEER_CHANNEL` 索引行，在 `apifull_channel` 和原生 `channels` 中都没有对应频道行。没有使用或修改这两条数据；其解析结果未验证，需作为独立生产数据完整性缺口处理。
- 本次联系人 probe 只覆盖 1 个联系人和 offline presence；空 presence、在线状态、多联系人、跨会话更新和进程重启后的持久性仍未验收。证据已更新到 `LAYER229_METHOD_LEDGER.csv`。

## 2026-10-07：频道 sendMedia 写入 canonical 台账

- `messages.sendMedia` 对 `InputPeerChannel` 增加单条媒体写入路径：验证 peer、成员与频道写权限，调用现有 Media provider 解析支持的 `InputMedia`，随后将媒体、实体和键盘写入 APIFull 频道消息 canonical 行及差分事件。随机 ID 的内容指纹取自客户端原始请求，避免媒体解析产生的内部 ID 变化影响重试冲突判断。即时 Updates、频道历史和 `updates.getChannelDifferenceV2` 共享持久化内容。
- 新迁移兼容 `apifull_channel_message.content_json` 已由基础迁移建立的情况，并为已有事件表补齐列；目标环境仍需应用 `migrate-20261008-channel-message-media.sql`。
- 受影响 Go 包的定向 `go build` 与 `git diff --check` 通过。未运行测试、未应用迁移，也未进行真实 session/Media provider/生产频道写入与读回；因此 `messages.sendMedia` 的生产验收级别不变。定时媒体和频道相册仍未实现。唯一方法状态以 `LAYER229_METHOD_LEDGER.csv` 为准。

## 2026-10-07：provider 部署凭证入口

- BFF 启动时支持通过 `TEAMGRAM_PAYMENT_PROVIDER_*` 注入支付 provider endpoint、访问密钥、响应签名密钥和超时；未配置 endpoint 或签名密钥时支付路径继续 fail-closed。`backend-deploy-r22-prod.yaml` 声明这些变量但不包含任何默认凭证。
- BFF 同样支持 `TEAMGRAM_CODE_*` 注入 SMS/Email provider、验证码端点、challenge secret、重试/超时、TTL、限流和尝试次数。challenge secret 和真实投递 provider 缺失时认证路径继续 fail-closed。
- TURN host、静态凭证/shared secret 和 TTL 通过现有 `TEAMGRAM_TURN_*` 入口覆盖；BFF 不再记录完整配置，避免日志携带 secret。相关构建与 Compose 配置检查通过；没有配置真实支付、短信/邮件或 TURN 凭证，也没有执行生产重启或业务验收，台账状态不提升。

## 2026-10-07：频道消息实时成员 fan-out

- 原生频道文本、单媒体和相册消息提交 canonical APIFull 存储后，现在通过 `SyncPushUpdatesIfNot` 将同一组 `Updates` 发给频道可见成员，并排除发送请求的永久授权 key。有效 roster 来自 APIFull 频道成员表；当前被禁止查看频道的成员不会收到更新。Updates 同时携带频道实体。
- 任一 Sync 投递失败会返回给客户端；消息已提交，重试通过 `random_id` 与请求指纹复用原消息并再次尝试 fan-out。投递按成员逐一发布，没有与消息写入同事务的独立 fan-out outbox；若进程中断且客户端不重试，在线投递可能不完整，持久化 channel difference 仍可用于追赶。
- `go build ./app/bff/messages/internal/core ./app/bff/apifull/channelview` 通过。新增的隔离 MySQL fan-out 用例在当前环境编译通过，但 `APIFULL_MYSQL_DSN` 未配置，数据库断言未执行；没有应用频道迁移或进行真实 session/生产验证，因此生产验收级别不变。`LAYER229_METHOD_LEDGER.csv` 是这三种发送方法唯一状态来源。
