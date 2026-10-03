# Layer 229 API 功能验收

## 2026-10-03 密码邮箱 provider 接线

- APIFull 的 `account.resendPasswordEmail` 和 `account.confirmPasswordEmail` 已接入共享 Redis challenge 与 `Code.EmailProvider`。重发只在 provider、Redis 和限流检查成功后保存一次性 challenge ID；确认使用原子消费，成功后清除 pending 状态；取消会撤销未消费 challenge。
- `bff` 将 `Code` 配置传入 APIFull，空配置仍失败关闭。新增路径只完成组件接线，未连接真实邮件 endpoint/Redis，也未改生产数据库或重启 `backend-backend-1`。

## 2026-10-03 临时授权密钥输入门禁

- `auth.bindTempAuthKey` 在 BFF 层现在拒绝空请求、短密文、非块对齐密文和缺失 authsession provider，再把合法帧交给 authsession 的永久/临时密钥事务绑定。定向输入测试和 authorization 包测试通过；真实临时密钥握手仍未做 session→authsession 端到端验收。

## 2026-10-03 反应举报接入持久化接收队列

- `messages.reportReaction` 不再只返回 `METHOD_NOT_IMPL` 或写入无人消费的进程 KV；在 APIFull 数据库可用时，它校验消息 ID、目标 peer 和反应方 peer，并通过 `apifull_report` 的幂等接收表持久化请求。
- 没有把这项提升为已实现：反应举报的审核/处置消费者仍不存在，数据库不可用时继续失败关闭；本轮只做 `go test -c ./app/bff/apifull/internal/core`、`gofmt` 和 `git diff --check`，未连接生产库或隔离 MySQL。
- 同一接收路径还覆盖 `channels.reportSpam`、`messages.reportReadMetrics`、`messages.reportMusicListen` 和 `stories.report`；台账将这四项从静态缺口调整为组件部分实现，生产验收仍未通过。
- 当前 CSV 仍为 813 个唯一方法：`KNOWN_INCOMPLETE_STATIC=316`、`PARTIAL_COMPONENT_VERIFIED=135`、`PARTIAL_SESSION_VERIFIED=150`、`ROUTED_NOT_FUNCTIONALLY_ACCEPTED=129`、`PARTIAL_ISOLATED_DB_VERIFIED=52`，其余为隔离组件/数据库、传输包装或会话分发待验收项；生产级完整验收仍为 `0/813`。

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
- `channels.toggleAntiSpam` 与 `channels.reportAntiSpamFalsePositive` 只保存请求，不更新反垃圾配置或审核状态。
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
- **事实核查、快捷回复与 Business Bot：** `messages.editFactCheck`、`deleteFactCheck`、`getFactCheck` 只按调用用户在 APIFull 本地读写，不确认消息存在或编辑权限，也不通知消息服务。`messages.getQuickReplies` 和 `getQuickReplyMessages` 现在通过 `MessageGetUserMessage` 读取真实消息对象；快捷回复关联仍是 APIFull 本地状态，`sendQuickReplyMessages` 和 `deleteQuickReplyMessages` 仍缺少消息写入/删除流程。`account.updateConnectedBot` / `getConnectedBots` 仅有本地连接配置，未接到业务消息投递；`getBotBusinessConnection` 固定空，暂停/禁用设置只覆盖单条不被消费的 JSON。`ephemeral.reportMessage` 和 `getCallbackAnswer` 也只写入/读回每用户一条 note，没有举报或 callback 查询处理。
- **AI 文本、支持信息与广告：** `messages.summarizeText`、`translateRichMessage`、`composeRichMessageWithAI` 都返回原文或原 rich-text，没有摘要、翻译或生成服务。`messages.setTyping` 已补齐用户、普通群和 APIFull 频道的成员校验与 `UpdateChannelUserTyping` 推送；跨用户隔离会话已通过，生产双用户投递和重启持久性仍未验收。`help.getSupport`、`getSupportName`、`getInviteText` 固定空；`getAppUpdate` 不查询更新源而始终返回无更新。赞助消息的 `contacts.getSponsoredPeers`、`messages.getSponsoredMessages` 固定空，`account.toggleSponsoredMessages` 和 `channels.restrictSponsoredMessages` 只写本地 flag，view/click/report 只记本地事件，不接广告库存、分析或审核服务。
- **Passkey：** `account.initPasskeyRegistration`、`registerPasskey` 只处理挑战并存 credential ID，没有校验 attestation 或保存公钥；`getPasskeys`、`deletePasskey` 只管理这份本地 ID 列表。`auth.initPasskeyLogin` 使用空凭证白名单和 `localhost` RP；`auth.finishPasskeyLogin` 不验证 assertion，消费挑战后固定返回 `AUTH_BYTES_INVALID`，不会创建授权会话。
- **媒体与搜索：** `messages.getDocumentByHash` 校验参数后固定返回 `DocumentEmpty`，`help.getCdnConfig` 固定报 `CDN_METHOD_INVALID`，`messages.receivedMessages` 校验认证、请求构造器和 `max_id` 后在缺少回执 provider 时返回 `METHOD_NOT_IMPL`。`contacts.getTopPeers` 用用户 ID 合成排序分数且跳过非联系人分类；`messages.getRecentLocations` 把读取失败伪装成空结果并没有回填频道；`getSearchResultsPositions` 把当前页索引当全局位置；`channels.checkSearchPostsFlood` 永远给固定免费额度，不执行限流。
- **频道成员与导出：** `channels.getParticipants` / `getParticipant` 只把本地频道创建者当成员，无法列出普通成员或真实管理员。`account.initTakeoutSession` / `finishTakeoutSession` 只开关本地 session，不控制导出；`messages.getSplitRanges` 与 `channels.getLeftChannels` 在认证后 fail-closed 为 `METHOD_NOT_IMPL`，未实现 takeout 分区或退出频道历史，也不再伪造空结果。
- **Bot 与 Mini App：** `bots.createBot`、`getAdminedBots`、`checkUsername` 使用本地合成 Bot / 用户名键，`exportBotToken` 返回空 token；`getAccessSettings` 固定空，`editAccessSettings`、`setJoinChatResults`、两种默认管理员权限及 `setCustomVerification` 只保存请求或描述；`getBotRecommendations`、`getPopularAppBots` 固定空。`bots.sendCustomRequest` 回显输入 JSON，`answerWebhookJSONQuery` 与 `help.setBotUpdatesStatus` 只保存请求。`messages.requestWebView`、`requestSimpleWebView`、`requestAppWebView`、`requestMainWebView`、`requestChatJoinWebView`、`prolongWebView`、`sendWebViewResultMessage`、`sendWebViewData`、`getBotApp`，以及 `bots.invokeWebViewCustomMethod`、`checkDownloadFileParams`、`requestWebViewButton`、`getRequestedWebViewButton` 都是 URL 回显、本地记录或固定空值，未建立已授权的 WebView 会话或 Bot 调用链。常规 Bot 命令和菜单按钮本地读写不在此缺口列表。
- **付款、Stars 与预付 Giveaway：** `payments.getPaymentForm`、`sendPaymentForm`、`assignAppStoreTransaction`、`assignPlayMarketTransaction` 对有效凭证均返回 `PAYMENT_UNSUPPORTED`；收据不校验，Premium/权益不发放。`getPaymentReceipt` 只读本地合成记录，`validateRequestedInfo`、`getSavedInfo`、`clearSavedInfo` 在鉴权和必要的请求形状校验后返回 `METHOD_NOT_IMPL`，不再读写本地支付信息。`getBankCardData` 固定空。`payments.getStarsTopupOptions` 和 `getStarsGiftOptions` 固定空；`sendStarsForm` 只扣内部 Stars 余额并记本地流水；`refundStarsCharge` 对未知 charge 以 0 金额成功返回。Stars 订阅四个读写方法仅维护每用户一个本地订阅记录，没有续费或权益生命周期。`getPremiumGiftCodeOptions`、`getGiveawayInfo`、`launchPrepaidGiveaway`、`getStarsGiveawayOptions` 只保存一个 giveaway ID 并据此合成字段。
- **一对一 VoIP 与群通话：** `messages.deletePhoneCallHistory` 只存请求并返回 0 条受影响记录；`phone.requestCall`、`acceptCall`、`confirmCall`、`receivedCall`、`discardCall` 使用本地合成 ID / 状态，没有呼叫对端路由或信令投递；`setCallRating`、`saveCallDebug`、`sendSignalingData`、`saveCallLog` 只存请求。群/会议创建、加入/离开、参与者读取等方法只操作 APIFull 本地成员记录，没有通话控制面和跨客户端更新；流媒体 URL/频道只由本地参与者及 relay 配置构造，`phone.getGroupCallStars` 硬编码 0 星并把参与者当 donor。已在台账标为缺口的方法包括 `phone.createGroupCall`、`joinGroupCall`、`leaveGroupCall`、`getGroupCall`、`getGroupParticipants`、`checkGroupCall`、`getGroupCallJoinAs`、`getGroupCallStreamChannels`、`getGroupCallStreamRtmpUrl`、`createConferenceCall`、`deleteConferenceCallParticipants`、`getGroupCallStars`；现有已标缺口的邀请、录制、设置、屏幕共享、广播及通话消息方法继续保留原记录。

## 本轮第三次静态复核

本轮仍只读源码，没有运行测试、调用服务或连接数据库。源码明确显示下列 158 个路由方法只有本地模拟、固定空结果/固定零值，或没有执行协议要求的业务副作用，因此从“仅确认路由”改列为“静态确认不完整”。“不完整”不表示每个方法都完全空实现；具体证据在台账逐项列出。

| 功能 | 方法 |
| --- | --- |
| 限时动态 | `stories.canSendStory`、`sendStory`、`editStory`、`deleteStories`、`togglePinned`、`getAllStories`、`getPinnedStories`、`getStoriesArchive`、`getStoriesByID`、`toggleAllStoriesHidden`、`readStories`、`incrementStoryViews`、`getStoryViewsList`、`getStoriesViews`、`exportStoryLink`、`report`、`activateStealthMode`、`sendReaction`、`getPeerStories`、`getAllReadPeerStories`、`getPeerMaxIDs`、`getChatsToSend`、`togglePeerStoriesHidden`、`getStoryReactionsList`、`togglePinnedToTop`、`searchPosts`、`createAlbum`、`updateAlbum`、`reorderAlbums`、`deleteAlbum`、`getAlbums`、`getAlbumStories`、`startLive`。实现只维护调用者本地的合成记录，没有媒体存储、跨用户故事读取/投递或实时通话服务。 |
| 贴纸集和贴纸目录 | `messages.getStickers`、`getAllStickers`、`getStickerSet`、`installStickerSet`、`uninstallStickerSet`、`reorderStickerSets`、`getFeaturedStickers`、`readFeaturedStickers`、`getRecentStickers`、`saveRecentSticker`、`clearRecentStickers`、`getArchivedStickers`、`getMaskStickers`、`getAttachedStickers`、`getFavedStickers`、`faveSticker`、`searchStickerSets`、`toggleStickerSets`、`getOldFeaturedStickers`、`searchEmojiStickerSets`、`getMyStickers`、`searchStickers`；`stickers.createStickerSet`、`removeStickerFromSet`、`changeStickerPosition`、`addStickerToSet`、`setStickerSetThumb`、`checkShortName`、`suggestShortName`、`changeSticker`、`renameStickerSet`、`deleteStickerSet`、`replaceSticker`。目录/媒体是合成或固定空值，多数写方法只存请求、不改变读取路径。 |
| 自定义表情和关键词 | `account.getDefaultProfilePhotoEmojis`、`getDefaultGroupPhotoEmojis`；`messages.getCustomEmojiDocuments`、`getEmojiStickers`、`getFeaturedEmojiStickers`、`searchCustomEmoji`、`getEmojiKeywords`、`getEmojiKeywordsDifference`、`getEmojiKeywordsLanguages`、`getEmojiURL`、`getEmojiGroups`、`getEmojiStatusGroups`、`getEmojiProfilePhotoGroups`、`getEmojiStickerGroups`。没有服务端表情目录；关键词只从当前用户自建贴纸取值，语言和 URL 接口为空。 |
| 主题 | `account.uploadTheme`、`createTheme`、`updateTheme`、`saveTheme`、`installTheme`、`getTheme`、`getThemes`、`getChatThemes`、`getUniqueGiftChatThemes`、`messages.setChatTheme`。上传通过 DFS；create/update 仅接受调用者自己上传且已持久化的真实 Document，并保存主题元数据及可表示设置；全局主题目录和 chat-theme/unique-gift provider 仍未接入。 |
| 语言包 | `langpack.getLangPack`、`getStrings`、`getDifference`、`getLanguages`、`getLanguage`。翻译值回显请求 key/code，语言列表硬编码，差分版本不递增。 |
| SMS Jobs | `smsjobs.isEligibleToJoin`、`join`、`leave`、`updateSettings`、`getStatus`、`getSmsJob`、`finishJob`。只有本地标记/计数，没有任务分配或短信服务。 |
| Boosts | `channels.setBoostsToUnblockRestrictions`；`premium.getBoostsList`、`getMyBoosts`、`applyBoost`、`getBoostsStatus`、`getUserBoosts`。Boost 存为单个本地记录，数量与 ID 合成，没有扣除库存或修改频道状态。 |
| 游戏分数 | `messages.setGameScore`、`setInlineGameScore`、`getGameHighScores`、`getInlineGameHighScores`、`getEmojiGameInfo`。分数表按请求中的 peer/message ID 本地存储，没有验证游戏消息/权限；emoji game 忽略目标参数。 |
| 壁纸 | `account.getWallPapers`、`getWallPaper`、`uploadWallPaper`、`saveWallPaper`、`installWallPaper`、`resetWallPapers`、`getMultiWallPapers`、`messages.setChatWallPaper`。saved 列表仍是本地引用；上传的真实 DFS `Document` 现在保存到调用者自己的上传目录，`getWallPaper`/`getMultiWallPapers`/保存安装可读回该文档，但没有全局壁纸目录；聊天背景方法已写入 Dialog 的 peer 状态。 |
| 建议、深链和 Bot 信息 | `messages.getSuggestedDialogFilters`（固定空列表）；`help.getDeepLinkInfo`、`messages.startBot`、`help.getRecentMeUrls`（无后端数据源，现明确返回 `METHOD_NOT_IMPL`）；`help.dismissSuggestion`（固定成功、不记录）；`bots.getBotInfo`（忽略 bot/lang_code，总是读当前用户本地 Bot）。 |
| Premium 商店门面 | `help.getPremiumPromo`（硬编码“此服务器不提供 Premium”，无套餐）；`payments.canPurchaseStore`（忽略 purpose，只看本地 Premium 标记，没有商店/provider 检查）。 |
| 表情状态和颜色 | `account.updateEmojiStatus`、`getDefaultEmojiStatuses`、`getRecentEmojiStatuses`、`clearRecentEmojiStatuses`、`getChannelDefaultEmojiStatuses`、`getChannelRestrictedStatusEmojis`、`getCollectibleEmojiStatuses`；`account.updateColor`、`getDefaultBackgroundEmojis`、`channels.updateColor`、`channels.updateEmojiStatus`；`bots.updateUserEmojiStatus`、`toggleUserEmojiStatusPermission`。状态/颜色只存调用者本地单值，没有频道/目标用户写入和同步；recent 接口复用当前状态记录，clear 会把当前状态清空。 |
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
- `messages.searchGlobal` 现在只接受后端能表达的首屏、无筛选请求；缺少认证或 message/user/chat provider 时会在访问前失败关闭。文件夹、筛选、日期、非空游标和其它不支持的字段会明确返回 `METHOD_NOT_IMPL`；搜索 DAO 错误、用户/群对象 hydration 错误会传播，缺少本地频道对象时也明确失败。消息服务仍没有 peer-local 分页游标或权威原生频道 hydration。`messages.searchSentMedia` 会校验认证、filter 构造器和 limit 边界，bot 或 malformed 请求会失败关闭；合法请求仍因缺少 all-dialog sent-media provider 返回 `METHOD_NOT_IMPL`。现有 global text search 不能保证 sent-media filter 完整性或跨 dialog 分页，因此不能作为替代。`channels.searchPosts` 也因缺少符合请求语义的后端而失败关闭；`messages.search` 和 `messages.getSearchCounters` 现在传播后端错误，但仍只支持部分过滤器。
- APIFull 的 peer、profile photo、消息、频道举报和 read-metrics/music-listen 提交方法现在鉴权后返回 `METHOD_NOT_IMPL`，不再写入无人消费的本地 report blob 或 view counter。Layer 229 的 `stories.report` 也不再修改个人故事存储或伪造 `ReportResult`。Stars 状态、交易记录和按 ID 查询同样不再返回本地模拟数据，缺少权威 Stars 账本时返回 `METHOD_NOT_IMPL`。这些改动只证明假成功已被移除，不代表相应业务功能已经实现。
- 为与新行为一致，更新了举报和 story 举报的旧断言；本轮只做代码审查、`gofmt` 和差异空白检查，未运行测试、连接数据库或服务，也未部署或重启进程。完整的短信登录、受信 API 凭证校验、Stories/Stars/举报提供方、全局 sent-media 查询、原生频道搜索与分页仍是待补缺口。


## 本轮第十一次复核：授权撤销、群邀请、搜索与联系人令牌

- 重新比较 `../telegram-api/methods.json` 与台账：813 个唯一方法，缺失/多余均为 0。当前审计状态列为 466 项已知静态缺口、298 项仅确认路由、13 项隔离库验收、13 项部分隔离库验收、8 项隔离组件验收、2 项部分组件验收、2 项 session 手动分发待验收及 11 项传输包装。生产验收列仍为 0/813 完整端到端通过；457 项被实现缺口阻塞，297 项未验收，其余只有隔离库、组件或 core 层证据。
- `messages.search` 的消息 provider 现在在 DAO 前拒绝空查询、非法 peer 和负 limit，并传播 hashtag/全文 DAO 错误，同时使用规范化 offset/limit；voice 与 round-video 过滤器映射到消息服务支持的 media type 9 和 11。BFF 映射测试及生成 gRPC 客户端→消息服务→DAO→隔离 MySQL 测试通过，并确认数据库错误会传播。mentions、geo、contacts 过滤器及完整 query/pagination 语义仍不支持，因此该方法仍列为有实现缺口。
- `messages.exportChatInvite` 与 `messages.getExportedChatInvites` 的底层 chat gRPC 流程现在从调用元数据校验发起者身份、群成员和邀请权限。隔离 MySQL 测试覆盖导出、持久化字段、列表读回、非成员/普通成员/跨管理员权限拒绝以及缺失身份拒绝；fixture 行已清理。BFF/session、事件投递和实体 hydration 仍未验收。
- `auth.resetAuthorizations` 和 `account.resetAuthorization` 的底层 authsession gRPC→MySQL 流程已通过隔离审计库测试。覆盖保留当前 session、用户隔离、按 hash 撤销、墓碑状态、缓存失效，并确认返回永久 auth key ID。测试发现并修正了把已撤销永久 key ID 错映射成临时 key ID 的问题。BFF/session 更新投递与进程重启未验收。
- `contacts.exportContactToken` 现在生成 24 小时随机 token URL，并把 owner/expiry 存入 APIFull 持久层；`contacts.importContactToken` 校验缺失、未知、过期、自身及失效用户 token，解析用户、写入联系人关系并返回实际 User。核心 MySQL 往返、拒绝分支和写失败传播测试通过；UserClient 使用替身，临时审计 schema 在测试后移除，因此只记部分隔离库证据。
- 本轮只使用 `layer229-audit-mysql` 中的隔离审计 schema 和 bufconn gRPC 测试服务器；临时数据库账号按库授权并在结束后删除。没有连接 `teamgram` 默认业务库、向运行中的 backend 服务发 RPC、重启服务或执行破坏性 Layer 229 方法。

## 本轮第十二次复核：消息编辑、在线状态与联系人添加

- 重新从 `../telegram-api/methods.json` 展开 813 个方法，与台账逐项对比：813 个唯一名称完全匹配，无缺失或多余。生产级 BFF/session→业务服务→持久化/provider→读回和权限验证仍为 **0/813**。
- `messages.getMessageEditData` 的频道分支现在读取 APIFull 存储的真实频道文本消息，验证 access hash、成员关系、创建者权限和消息存在性；caption 目前为 false。`TestHistoryAndEditDataCheckInputPeerAuthorization` 在隔离 `teamgram_audit` 通过。原生频道消息存储、媒体 caption 和完整 session 链路未覆盖，因此只记部分隔离库证据。
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
