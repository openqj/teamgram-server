# Teamgram-Server 性能改动与验证状态

**优化日期:** 2026-10-16  
**目标:** 降低已确认的服务端开销；当前没有 Telegram 官方基准数据  
**数据库:** PostgreSQL 18  
**API 层级:** Layer 229 (Telegram User API)

---

## 执行摘要

仓库中有若干性能相关代码改动；本轮首次补充了同参数的隔离 fan-out 对照结果，但覆盖范围仍有限。本文不把估算值当作实测结果，也不据此声称达到 Telegram 官方性能。

**关键成果:**
- ✅ session 更新推送改为有并发上限的并行分发
- ✅ auth key 查询改为一次 PostgreSQL JOIN，替代两次 DAO 查询
- ✅ Session stream 客户端已接入服务端默认策略兼容的 keepalive
- ✅ Kafka Sync 安全提交批量从 8 条提高到 64 条，并增加消费队列、分区滞后和 in-flight 指标
- ✅ Sync PostgreSQL pool 增加连接占用、空池等待和取消 acquire 观测
- ✅ Sync 回执完成更新按最多 64 条、2ms 窗口合并到一个显式事务
- ⚠️ auth key LRU 缓存未实现；Inbox 异步投递不能归因于本轮改动
- ⚠️ 索引建议未通过实际查询计划验证，未部署
- ⚠️ 运行配置来自 `teamgramd/etc2`；连接池还包含代码创建的额外池，不能只看 YAML 总和

## 2026-10-16 隔离 Fan-out 实测

本轮使用相同参数运行了优化前报告和安全提交批量版本：5000 个在线 Layer 229 客户端、2 个 backend、6 个 `Sync-T` 分区、每个目标 100 个断线恢复客户端，同时覆盖一个 broadcast channel 和一个 megagroup。原始证据保存在：

- `/tmp/teamgram-group-channel-fanout-5000-20261016-r6.json`
- `/tmp/teamgram-group-channel-fanout-5000-20261016-safe-commit64.json`

新运行结果为 `BUDGET_EXCEEDED`，不是 `PASS`：5000 个客户端全部认证并保持连接，两个目标均收到 5000/5000 条消息，无重复，PTS 连续、历史可读、恢复和 outbox 均完成；但 1 秒 ACK 和 2 秒全量实时送达预算没有达到。

| 指标 | 原报告 | 安全提交批量 | 说明 |
| --- | ---: | ---: | --- |
| Sarama record age P95 | 16.384–32.768s | 0.064–0.128s | 交付给 handler 前的 Kafka 记录年龄，服务端指标 |
| offset commit P95 | 0.256–0.512s | 0.256–0.512s | 本轮没有证明提交耗时下降 |
| channel 客户端可见 live P95 | 30.933s | 2.906s | 5000 个本机 GramJS 客户端观察到的延迟 |
| megagroup 客户端可见 live P95 | 27.772s | 2.471s | 仍超过 2 秒预算 |
| channel ACK | 1.903s | 3.042s | 仍超过 1 秒预算，且本轮没有改善 |
| megagroup ACK | 2.089s | 2.348s | 仍超过 1 秒预算，且本轮没有改善 |

新报告的服务端分解指标显示 session 到 gateway ACK P95 约 25ms、gateway 写队列等待 P95 约 5ms；因此 2.47–2.91 秒的客户端可见延迟不能直接归因于 gateway 写入。5000 个 GramJS 客户端共用一台 macOS 探针主机，客户端事件循环和本机资源仍是未隔离变量。

验证环境还暴露了连接容量配置风险：第一次启动使用旧派生镜像中的 `sync MaxConns: 128 / MinConns: 16` 和 PostgreSQL `max_connections=300`，两台 backend 在启动阶段耗尽连接并返回 `too many clients already`。将隔离数据库上限临时提高到 1000 后，测试完成；后续同参数 A/B 已将当前工作树的 `teamgramd/etc2/sync.yaml` 调整为 `MaxConns: 16 / MinConns: 1`，生产部署仍需按实际镜像、服务进程和副本数重新计算连接预算。

这次验证使用的是 `teamgram-reliability:20261016-safe-commit64-local` 派生镜像（仅将 Linux ARM64 `sync` 二进制写入已有隔离镜像），不是完整生产镜像构建；两份报告的镜像来源也不是同一个干净 release 构建，不能把全部差异归因于单一改动。测试没有生产流量或 Telegram 官方基准，因此结果只能证明本地隔离场景中的相对变化，不能推导生产容量、Telegram 对标或 3–5 倍吞吐量。

另外运行了四进程 listener diagnostic：5000/5000 收到且无重复，数据库 outbox 完成，但仍为 `BUDGET_EXCEEDED`；四个 Node shard 的 event loop P95 约 22ms，客户端可见 P95 约 4.1–4.4s，ACK 约 3.1–3.6s。该结果排除了“单个 Node 事件循环完全阻塞”这一单一解释，但 diagnostic 仍共享本机 CPU、网络和 Docker 资源，不能替代服务端 trace 或生产压测。原始证据：`/tmp/teamgram-group-channel-fanout-5000-20261016-multiprocess4.json`。

## 2026-10-16 完整源码镜像复测与数据库分解

为确认前一轮临时派生镜像没有掩盖当前源码行为，使用当前工作树完整构建了 Linux ARM64 镜像：

- 镜像：`teamgram-reliability:20261016-current-full`
- 镜像 manifest：`sha256:5a4f376f7f5ba9279a35e3a52ad0b0eff163e503123569c59a43975822e37f47`
- PostgreSQL：18，实验上限 `max_connections=1000`
- 原始证据：`/tmp/teamgram-group-channel-fanout-5000-20261016-current-full-r2.json`

同样的 5000 客户端、双 backend、6 个 Sync 分区实验仍为 `BUDGET_EXCEEDED`。所有客户端都收到消息且无重复，PTS、历史、恢复和 outbox 均正确；客户端可见延迟仍未达到预算：

| 目标 | ACK | live P50 | live P95 | live P99 | 2 秒内收到 |
| --- | ---: | ---: | ---: | ---: | ---: |
| channel | 1737ms | 2216ms | 5025ms | 6040ms | 2199/4900 |
| megagroup | 1073ms | 1730ms | 3554ms | 7035ms | 2928/4900 |

新增 `teamgram_apifull_channel_persistence_seconds` 指标对同一实验的 canonical 与 outbox 阶段进行了实测。每个目标只有 1 次消息事务和 1 次 outbox 写入（5000 个 recipient 在该事务内批量插入）：

| backend | message_tx | outbox_write |
| --- | ---: | ---: |
| backend-1 | 28.8ms | 13.0ms |
| backend-2 | 91.4ms | 12.6ms |

该结果说明这次实验中 PostgreSQL canonical/outbox 写入本身远低于 1 秒 ACK 预算，不能把 1–1.7 秒 ACK 或 3.5–7.0 秒 live 延迟归因于这段事务。剩余瓶颈仍需通过 request-to-response trace、Sync 消费队列、gateway 写入和探针主机资源隔离继续定位；该指标只覆盖后端持久化阶段，不等同于端到端 ACK。

## 2026-10-16 ACK 路径异步 fan-out 修复

进一步检查发现 `messages.sendMessage` 在返回 RPC 前同步调用了完整的
`DeliverPendingChannelUpdates`，所以 ACK 实际等待了所有 recipient 的 Sync
RPC。现在普通文本、单媒体和媒体相册的 channel 发送都只等待 canonical/outbox
持久化；后台 dispatch 使用最多 2 个并发槽位，槽位满或 dispatch 失败时由同一
outbox reconciler 重试。返回给客户端的 `Updates` 在启动后台任务前完成校验和
`Chats` 填充，避免返回对象与后台任务并发修改。

使用完整源码镜像复测：

- 镜像：`teamgram-reliability:20261016-async-channel-delivery`
- 镜像 manifest：`sha256:e065a47743aaf55a7982a83a42816470d792e7c68ddc9e3530fe01d474d33a1f`
- 原始证据：`/tmp/teamgram-group-channel-fanout-5000-20261016-async-channel-delivery.json`

| 目标 | ACK | live P50 | live P95 | live P99 | 收件人/恢复 |
| --- | ---: | ---: | ---: | ---: | --- |
| channel | 85ms | 1966ms | 4704ms | 5707ms | 5000/5000，恢复 100/100 |
| megagroup | 38ms | 1434ms | 3854ms | 5346ms | 5000/5000，恢复 100/100 |

两个目标均无重复、PTS 连续、历史可读、outbox 完成，但 2 秒全量实时送达
预算仍未达标。该改动解决了 ACK 被 fan-out 阻塞的问题，不等于实时 fan-out
吞吐已达到生产目标；后续瓶颈集中在 Sync/Kafka handler queue 和探针主机的
客户端资源。

## 2026-10-16 Sync PostgreSQL 连接池 A/B

为验证 `sync` 连接池是否让 64 个 handler 在数据库阶段排队，使用同一完整
源码构建流程、同一 5000 客户端负载、双 backend、6 个 `Sync-T` 分区和每目标
100 个恢复客户端，对比 `MaxConns: 8` 与 `MaxConns: 16`。所有四轮均为
`BUDGET_EXCEEDED`，但四轮都通过了完整性验收：5000/5000 唯一收件、无重复、
PTS 连续、历史可读、恢复 100/100、outbox 排空和连接清理。

原始证据：

- 8 连接基线：`/tmp/teamgram-group-channel-fanout-5000-20261016-async-channel-delivery.json`
- 8 连接基线复跑：`/tmp/teamgram-group-channel-fanout-5000-20261016-sync-pg8-handler64-r2.json`
- 16 连接候选：`/tmp/teamgram-group-channel-fanout-5000-20261016-sync-pg16-handler64.json`
- 16 连接候选复跑：`/tmp/teamgram-group-channel-fanout-5000-20261016-sync-pg16-handler64-r2.json`

| MaxConns | 轮次 | channel live P95 | megagroup live P95 | `prepare_if_not` + `mark_complete_if_not` 累计秒数 |
| ---: | --- | ---: | ---: | ---: |
| 8 | r1 | 4704ms | 3854ms | 1692s |
| 8 | r2 | 4615ms | 3779ms | 1530s |
| 16 | r1 | 4083ms | 4420ms | 1242s |
| 16 | r2 | 3931ms | 3476ms | 1134s |

`MaxConns: 16` 两轮的数据库阶段累计秒数都较低，channel 的实时 P95
也较低；megagroup 只有第二轮明显改善。四轮的 record age P95 和 handler
queue P95 仍分别落在 2.048–4.096 秒和 1.024 秒桶，说明连接池不是当前
Kafka/客户端尾延迟的唯一瓶颈。当前源码保留 `MaxConns: 16` 作为经过本地
A/B 的候选值，但不能据此宣称达到 2 秒实时预算、生产容量或 Telegram 官方
水平。生产部署前仍需按副本数核算 PostgreSQL 连接预算并观察池等待、CPU、
record age 和 outbox 正确性。

本轮补充了 Sync 专用 PostgreSQL pool 观测：`teamgram_sync_postgres_pool_connections`
按 `acquired`、`idle`、`total`、`constructing` 暴露当前连接数，另外记录
`max_connections`、成功 acquire 次数、成功 acquire 累计耗时、空池等待次数、
空池累计等待时间和取消 acquire 次数。指标在进程启动时采样，此后每 5 秒更新；
累计值是 pgxpool 进程内快照，进程重启后归零。压测时应同时观察：

```promql
teamgram_sync_postgres_pool_connections{state="acquired"}
teamgram_sync_postgres_pool_connections{state="idle"}
teamgram_sync_postgres_pool_max_connections
delta(teamgram_sync_postgres_pool_empty_acquire_count[5m])
delta(teamgram_sync_postgres_pool_empty_acquire_wait_seconds[5m])
```

如果空池等待与 `acquired / max_connections` 同步升高，才有依据继续评估
receipt 批量化或连接预算；如果空池等待接近零而 `mqconsumer_handler_queue_seconds`
和 record age 仍高，应优先检查 Kafka 调度、handler CPU、gateway 写队列和探针
资源，不再仅凭连接数加大并发。

## 2026-10-16 Sync 回执完成批处理 A/B

连接池观测显示 16 连接配置下，单条 `MarkDeliveryComplete` 的事务会制造
大量 pool acquire 等待。现在 Sync 在 fan-out 成功后，将最多 64 条回执完成更新
在 2ms 窗口内合并到一个 PostgreSQL 事务；调用方仍等待事务提交，失败仍返回
Kafka handler 并触发原有重试。批处理按 group/topic/partition/offset 稳定排序，
避免同一用户的回执锁顺序不一致。`TEAMGRAM_SYNC_POSTGRES_MAX_CONNS` 只用于
运行时 A/B，默认仍为 16。

1000 用户、双 backend、6 个 `Sync-T` 分区的两个隔离报告：

- 批处理前：`/tmp/teamgram-group-channel-fanout-1000-20261016-sync-pool-metrics-r3.json`
- 批处理后：`/tmp/teamgram-group-channel-fanout-1000-20261016-sync-completion-batch.json`

| 指标（两台 Sync 合计） | 批处理前 | 批处理后 | 变化 |
| --- | ---: | ---: | ---: |
| pool acquire 次数 | 6004 | 3349 | -44.2% |
| 空池等待次数 | 4010 | 1687 | -57.9% |
| 空池累计等待 | 158.4s | 44.9s | -71.7% |

两轮均为 `PASS`：1000/1000 唯一收件、无重复、PTS 连续、历史可读、每目标
20 人差量恢复、outbox 排空和连接清理全部通过。批处理后的 channel live P95
为 328.5ms、megagroup 为 313.8ms；批处理前分别为 290.7ms 和 312.4ms，
没有稳定的端到端延迟下降，因此不能把 pool 等待下降写成客户端 P95 改善。

此前 32 连接 A/B 报告 `/tmp/teamgram-group-channel-fanout-1000-20261016-sync-pool32.json`
虽然降低了部分空池等待，但 live P95 没有稳定改善，默认值不提升到 32。当前
保留默认 16 和覆盖入口，生产仍需在固定资源的 5000 用户档复测后再核算连接预算。

## 2026-10-16 5000 用户批处理复测

使用当前工作树构建的批处理版本、双 backend、6 个 `Sync-T` 分区和每个目标
100 个断线恢复客户端运行了 5000 用户档，原始报告为
`/tmp/teamgram-group-channel-fanout-5000-20261016-sync-completion-batch.json`。
该轮结果是 `BUDGET_EXCEEDED`，不是实时达标：ACK 已回到 1 秒预算内，完整性和
恢复验收通过，但 2 秒内全量实时收件仍未达到。

| 目标 | ACK | live P50 | live P95 | live P99 | 2 秒内实时收件 | 最终唯一收件 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| channel | 49ms | 1,142ms | 4,628ms | 5,139ms | 3,919/4,900 | 5,000/5,000 |
| megagroup | 57ms | 1,192ms | 2,958ms | 3,868ms | 4,386/4,900 | 5,000/5,000 |

两类目标均无重复、PTS 连续、历史可读、outbox 排空，100/100 断线客户端均完成
差量恢复。两台 Sync 的最终 pool 快照为 16 个最大连接；累计 acquire 为
8,596 和 8,617，空池等待为 4,100 和 4,065，累计等待约 100.0s 和 113.1s。
这些数据说明批处理降低了 1000 用户档的 pool 压力，但在 5000 用户档没有把实时
尾延迟压到 2 秒，不能据此声称生产容量或 Telegram 对标性能。

## 2026-10-16 进一步实现与回归结论

- `delivery_completion_batch.go` 在同一显式事务内改用 pgx `SendBatch`，将最多 64
  条回执更新合并为一轮数据库往返；请求排序、`request_hash` 校验、`delivery_data`
  条件、提交失败重试语义保持不变。当前已有编译和聚焦回归，尚无新的固定资源
  fan-out 对照，因此不把它写成已验证的延迟收益。

随后用宿主机交叉编译的 Linux ARM64 `sync`/`gnetway` 二进制注入派生镜像
`teamgram-reliability:20261016-sendbatch-streamwriter`（ID
`sha256:d0b152b4b1961d05117e628ffd90919374c927a525c5caccc2924242934d978f`）
做了 1000 和 5000 用户隔离回归。1000 用户轮次为 `PASS`，channel/megagroup
live P95 为 318/303ms；5000 用户轮次保持完整收件、恢复、PTS、outbox 和清理
正确，但仍为 `BUDGET_EXCEEDED`，channel/megagroup live P95 为 5,042/2,173ms。
与已有轮次相比没有稳定的尾延迟改善，不能把 `SendBatch` 的单轮结果写成性能
提升。

本轮也短暂评估了 gateway stream 的有界 response writer，以减少 worker 持有
`stream.Send` 锁的时间；1000/5000 用户结果没有证明收益，因此没有保留这项
未经证明的并发改动。当前源码继续使用原有发送顺序和错误传播语义。

最新 5000 用户报告中的 gateway 写队列直方图单位是毫秒，P95 约 5ms；它不是已
确认的秒级主瓶颈。探针主机、Kafka handler 调度和端到端客户端事件循环仍需分别
隔离测量，不能只凭 gateway worker 解耦推导实时送达收益。

---

## Phase 1: 性能瓶颈分析

**分析范围:**
1. 消息发送路径 (messages.sendMessage)
2. 会话管理路径 (auth/session)
3. 更新投递路径 (sync/updates)

**待验证的问题假设:**

### 消息发送路径 (10 个问题)
- Issue #1: 串行身份解析 — 每次请求查询用户上下文
- Issue #4: 单条消息插入 — sendMultiMedia 无批量优化
- Issue #5: 串行用户数据获取 — 发送者/接收者信息顺序查询
- Issue #7: N+1 查询模式 — 100 人群聊需要 400+ 数据库查询
- Issue #8: 缺失索引 — user_messages 表全表扫描

上述查询数和延迟范围是待验证假设，并非从当前环境采集的 trace 或 benchmark；Telegram 数值也没有可核验来源。

### 会话管理路径 (3 个问题)
- Issue #11: 每次请求验证 auth key — 无缓存
- Issue #12: 缺失连接池 — 默认 4-10 连接
- Issue #13: 无权限缓存

### 更新投递路径 (5 个问题)
- Issue #14: 串行会话推送 — 5 会话 × 20ms = 100ms
- Issue #15: 状态查询阻塞
- Issue #16: 递归离线处理
- Issue #17: 无跨用户批量投递

**详细报告:** `/tmp/teamgram-performance-analysis.md`

---

## Phase 2: 高优先级代码优化

### 优化 1: 并行化会话更新推送

**修改文件:** [app/messenger/sync/internal/core/core.go](teamgram-server/app/messenger/sync/internal/core/core.go)

**改进:**
实现使用最多 8 个并发 worker 分发 session 目标。实际延迟取决于会话数量、服务端负载和网络；尚无对照压测。

### 优化 2: 合并 Auth Key 查询

**修改文件:** [app/service/authsession/internal/dao/postgres_runtime.go](teamgram-server/app/service/authsession/internal/dao/postgres_runtime.go)

**改进:** `keyInfo` 现在通过一次 JOIN 读取 auth key 和授权元数据。没有添加缓存；缓存命中率和延迟收益均未知。

### 优化 3: 持久 Inbox 投递

PostgreSQL 消息路径已有事务 outbox 和后台投递 worker，能把投递意图与消息持久化放在同一事务中。当前 diff 未证明这是本轮新增改动；旧路径仍有同步 RPC。需单独验证两种路径的行为和延迟。

## Phase 3: 基础设施优化

### 优化 1: PostgreSQL 连接池配置

Docker 启动脚本读取 `teamgramd/etc2/*.yaml`。字段 `MaxConns` 直接相加为 104，但这不是实际池上限。按 `runall-docker.sh` 的单实例源码展开，并按每个池的 `MaxConns` 全部同时使用计算：idgen 16、authsession 8、media 32、biz 的 5 个池 40、msg 的 2 个池 16、sync 16、bff 约 48、httpserver 约 32、session 约 16，合计约 **224**。这是配置上限，不是并发实测；BFF 的 process-wide APIFull persist 池只计一次，扩展副本时还要按副本数相乘。

Compose PostgreSQL 原先未配置 `max_connections`，运行中的 PostgreSQL 18 测试实例实际报告值为 100，低于上述单实例池上限。现在默认通过 `TEAMGRAM_POSTGRES_MAX_CONNECTIONS=256` 启动 PostgreSQL，并允许部署覆盖；可靠性实验保留独立的 `TEAMGRAM_RELIABILITY_PG_MAX_CONNECTIONS`。本地 backend 测试实例已在持久卷上执行可回滚的 `ALTER SYSTEM` 并重启验证，当前 `SHOW max_connections` 为 256；生产环境应通过 Compose 重建或受控重启使配置生效。256 只为单个 `runall-docker.sh` 实例预留约 40 个迁移、exporter、运维和保留连接，不能替代多副本容量规划。

重启后的初始本地快照看到 32 个客户端连接（包含当前观测用的 1 个 psql 会话，其余连接当时 idle），按 `max_connections - superuser_reserved_connections` 计算使用率约 12.65%；随后 exporter 只读实测看到 4 个客户端连接、可用连接利用率约 1.58%。这些都是瞬时观察值，不是峰值或生产负载证明。旧快照曾看到 25 和 32 个客户端连接，均只用于诊断。已为 `etc2` 中的 pgx 池设置服务级 `application_name`，APIFull SQL 池默认标为 `teamgram-apifull`，并为生产 postgres_exporter 增加按应用名、状态、`max_connections` 和实际连接使用率统计的指标；Prometheus 在可用连接使用率持续超过 80% 时告警。除 `sync` 池经过本节 A/B 验证调整为 16 外，其余池容量不因单一告警上调；部署后仍应观察连接分布和等待，再按真实并发及压测确定预算。

### 优化 2: gRPC Keepalive 配置

Keepalive 已应用在 `NewStreamingSession` 实际创建的双向 stream 连接上，间隔为 5 分钟，不允许无 stream 时 ping。它用于探测空闲连接故障，不缩短连接建立时间，也不证明连接复用率提升。原先 sync/messages client 中未接入连接创建路径的 keepalive helper 已移除。

### 优化 3: 数据库索引设计

原建议中的 `user_messages` 表在 PostgreSQL schema 中不存在，`user_pts_updates` 也没有 `deleted` 列。现有迁移已为 `messages`、`user_pts_updates` 和 auth key 建立访问索引。不要部署原建议 SQL；新增索引需先对照实际 DAO 查询、schema 和 `EXPLAIN (ANALYZE, BUFFERS)` 结果。

**详细报告:** `/tmp/teamgram-optimization-phase2.md`

---

## 性能结论

本轮已有可复现的隔离结果：安全提交批量使 Kafka record age 明显下降；异步 channel fan-out 又把客户端 ACK 从约 1–3 秒降到 28–85ms；`sync MaxConns: 16` 在两轮 A/B 中降低了数据库阶段聚合耗时，但没有消除 Kafka/客户端尾延迟。客户端可见 live fan-out 仍约 3.5–4.7 秒 P95，2 秒实时送达预算没有达标。消息发送 P50/P99、Auth Key 延迟、连接建立开销、吞吐量，以及与 Telegram 的比较仍未得到验证，应视为撤回的估算，不用于容量规划或上线承诺。

因此当前状态是：**值得保留异步 ACK、安全提交批量、`sync MaxConns: 16` 候选值和可观测性改动；不能把 5000 人 fan-out 标记为实时达标，也不能声称达到 Telegram 官方后端水平。** 后续压测仍需使用完整、同一来源构建的镜像，固定 CPU/内存/网络和客户端进程布局，并分别记录服务端 ACK、gateway queue、Sync record age 与客户端可见延迟。

当前本地 backend 测试栈的基础 Compose 未启动 Kafka，`messenger.sync` 会因 `kafka:9092` DNS 不可用退出并触发整组服务重启；该环境的健康检查可能短暂显示 healthy，但不能作为稳定性或性能基准。恢复 Kafka 服务或配置可达的 broker 后，才能运行后续消息和 fan-out 压测。

---

## 部署清单

### 已落地的代码改动
- ✅ 有上限的并行 session 更新推送
- ✅ 合并 Auth Key 查询
- ✅ PostgreSQL 事务 outbox 投递路径（其是否为本轮新增尚未证实）
- ✅ session stream keepalive 故障探测
- ⚠️ ACK 阻塞已通过异步 fan-out 修复并在本地隔离场景验证；live fan-out、吞吐量和生产容量仍未验证

### 暂不部署
- 原提议的索引 SQL 与当前 schema 不匹配
- 未经压测不得把 256 解释成吞吐量或延迟保证；多副本部署需重新核算并覆盖 `TEAMGRAM_POSTGRES_MAX_CONNECTIONS`

### 验证步骤

1. **代码审查**
```bash
git diff --check
git log --oneline -10
```

2. **编译验证**
```bash
cd teamgram-server
make build
```

3. **PostgreSQL 连接池验证**
```bash
rg -n "MaxConns|MinConns" teamgramd/etc2
```

4. 先在隔离环境跑兼容性和 PostgreSQL 集成检查，再用相同数据集、并发数和机器规格对比压测；未得到结果前不要发布量化收益。

---

## Layer 229 兼容性

本轮没有修改 TL schema 或公开方法签名。gRPC keepalive 仅作用于内部 session stream 传输。测试覆盖了 stream ACK、取消、并发以及服务端 request key 分组；本轮没有运行完整 Layer 229 客户端兼容套件或真实 PostgreSQL smoke，因此不能据此宣称端到端兼容验收完成。

---

## 下一步建议

### Phase 4: 性能验证与压测
1. 使用 Session Probe 框架进行 Layer 229 实时验证
2. 运行 RestartRecoveryTest 验证重启后状态一致性
3. 运行 MultiSessionUpdateProbe 验证跨会话 fan-out
4. 使用 wrk/k6 进行压力测试
5. 对比优化前后的性能指标

### Phase 5: 持续优化
1. 实施 prepared statements 优化高频查询
2. 优化 N+1 查询模式（批量预加载用户数据）
3. 实施 Redis 缓存层（用户信息、聊天信息）
4. 优化跨服务 RPC 调用（合并多次调用）

---

## 相关文档

- [性能瓶颈分析报告](/tmp/teamgram-performance-analysis.md)
- [Phase 1 代码优化报告](/tmp/teamgram-optimization-phase1.md)
- [Phase 2 基础设施优化报告](/tmp/teamgram-optimization-phase2.md)
- [Session 验证框架](docker/web-isolated/session-probe-framework.ts)
- [重启恢复测试](tools/restart-recovery-test.ts)
- [多会话更新验证](docker/web-isolated/multi-session-update-probe.ts)

---

**优化完成日期:** 2026-10-16  
**预计上线时间:** 待代码审查和压测通过后部署
