# 通信后端可靠性：本地隔离验收

本轮保留 Teamgram 的 MTProto 和 Telegram User API Layer 229 边界，以 PostgreSQL 18 为权威存储，补齐 Kafka 有界发布、投递观测、真实会话故障验收和数据库恢复演练。这里的门槛是本项目的本地验收预算，不是 Telegram 官方指标，也不代表生产 SLA。

## 实现与提交边界

- 生产 Kafka 创建点使用 `pkg/mqproducer`：每个 producer 最多 64 个等待最终结果的记录；调用等待最多 30 秒，网络和 metadata 操作 5 秒，重试最多两次；采用 `WaitForAll` 和按 key 分区。调用取消后，已接受记录仍占容量，直到后台收到最终 ACK/error，避免超时重试导致无界堆积。
- 各 producer 独立持有客户端，保留现有 method、业务 metadata 和 trace headers。`CloseAll` 并行排空、共享 30 秒预算；msg/sync/BFF 的 shutdown 等待为 35 秒，Compose 容器退出宽限为 45 秒。
- 既有 PostgreSQL 消息、update、`pts` 和 delivery intent 的事务边界保持不变。inbox 意图仅在消费者完成持久化和 update 发布后完成；broker ACK 不等于客户端收到。调用取消可能存在发送结果不确定，租约和幂等键仍是恢复依据。
- 有序 inbox 在上一条消费者确认前不能发布下一条。worker 在最近一秒内有投递进展时每 25ms 检查一次，空闲恢复一秒，避免原先每条消息额外等待一秒。保留租约、消费者确认与顺序屏障；繁忙阶段数据库查询频率会增加，需纳入部署容量预算。
- httpserver 从 `TEAMGRAM_POSTGRES_DSN` 加载实际数据库配置，启动日志不输出完整配置或 webhook 签名密钥。空 DSN/连接失败仍由现有构造器拒绝启动。

## 可重复执行

需要 Docker、Python 3、Go，以及已安装 `tsx` 的相邻 `telegram-tt-master`。无需生产账号、短信或 provider 凭据。两个账号与 256 字节 auth keys 仅在本次新数据库中生成；它们验证通信，不验证注册登录。

从包含 `teamgram-server` 和 `proto` 的工作区根目录构建全部后端二进制：

```sh
docker build --pull=false \
  -f teamgram-server/docker/reliability/Dockerfile.backend \
  -t teamgram-reliability:20261015 .
```

本轮最终回归使用已编译全部服务的验收镜像 `teamgram-reliability:20261015-acceptance`。它以基线运行时为底层，替换同一次 builder 生成的 `teamgramd/bin`、`etc2` 和启动脚本；运行前应记录 `docker image inspect` 的不可变 ID。

进入 `teamgram-server`，先查看计划，再执行：

```sh
python3 tools/postgres_recovery_probe.py
python3 tools/messaging_reliability_probe.py

python3 tools/postgres_recovery_probe.py --execute \
  --output /tmp/teamgram-postgres-recovery.json
python3 tools/messaging_reliability_probe.py --execute --go-tests \
  --image teamgram-reliability:20261015 \
  --gramjs-dir ../telegram-tt-master \
  --output /tmp/teamgram-messaging-reliability.json
```

每次自动使用全新 `teamgram-reliability-*` 项目、自有 network/volume 和 run 标签；拒绝复用既有资源，修改及清理前检查所有权。host 挂载均为仓库只读源文件，端口只绑定 `127.0.0.1`。PostgreSQL 恢复默认端口 55435；通信实验使用 PostgreSQL 55436、WebSocket 21543、Kafka 19094、Prometheus 19090，均可通过命令参数调整。端口占用直接失败。

执行结束即使验收失败也会清理本次资源，报告须同时查看 `status` 和 `cleanup.status`。没有 `--execute` 时只生成计划。现有共享项目、容器、数据目录和业务验收 wrapper 不在目标中。

## 消息验收断言

基线每方向 20 条顺序发送，共 40 条负载样本，另有新会话发送。记录 `performance.now()` 原始时间和双方状态，重新计算 nearest-rank p50/p95/p99；发送 RPC p95 ≤1000ms，recipient 实时 update p95 ≤2000ms，所有记录的 RPC 成功，双方 `pts` 连续且 `qts/seq/date` 不回退。该吞吐包含状态核对和等待，是探针实际速率，不能当作系统容量。

探针保留既有 `InitConnection`，显式封装 `InvokeWithLayer(229)`，校验 `messages.sendMessage` 构造器 `fef48f62`。在相同和新的 MTProto session 重放 `random_id`，检查没有新增 `pts` 或消息；核对双方历史各只有一个对应 message ID，最后分页删除 tag 消息并读回零剩余。新 session 复用 auth keys，不声称创建了新授权密钥。

重启阶段为 `before -> 独立 backend 容器替换 -> after`，保存业务 ID 后由新客户端读回、重放及继续收发。报告分别记录进程健康耗时和包含消息验证的总耗时；容器进程健康不等于业务恢复完成。

Kafka 故障阶段在存活的双账号 session 中使用文件屏障：

1. 客户端激活并取得基线，写 `ready`；runner 停止本次 Kafka 并确认退出，写 `stopped`。
2. 停机期间双向各发送一条，只等待标准持久化 ACK；禁止 `getState/getDifference/getHistory`。SQL 必须证明 2 条 sender 消息、0 条 recipient 消息和 4 条 pending 投递意图。
3. 必须观察到发送之后、当前时间之前的数据库采样，且 inbox pending ≥4；这证明 broker 中断未阻塞队列观测。runner 启动 Kafka 并等待 topic 协议查询成功，再写 `restored`。
4. 同一个 sender 对象、MTProto session ID 和在线连接必须先收到两条唯一实时消息，然后才读状态/历史。双方累计 `pts=baseline+2`，继续验证重放、新 session 和清理。恢复等待上限 120 秒；本地单 broker 的重试/租约恢复可能耗时数十秒。

## PostgreSQL 恢复断言

实验从空库运行所有带 checksum 的迁移，使用 PostgreSQL 18 的 `pg_basebackup`、`pg_verifybackup`、WAL 归档和命名 restore point：

- PITR 必须保留恢复点前已提交标记，排除恢复点后提交标记。
- 替换主库容器后已提交标记必须全部存在。
- 手工提升备用库前等待明确的复制回放屏障，再停止旧主库并确认禁用自动重启；提升后新主可写，旧主仍停止。

复制为异步，且演练有明确的停机前回放屏障；零标记丢失只覆盖该故障模型。这里没有自动仲裁、STONITH、跨机房或崩溃场景的零 RPO 保证。

## 指标与告警

`docker/reliability/prometheus.yml` 抓取 msg、sync、BFF 和 gateway。标准依赖 Compose 的 Prometheus 配置也加载 `reliability.rules.yml`。告警规则有 `promtool` 回归测试；本轮未配置对外告警通知渠道。

| 指标 | 含义 |
| --- | --- |
| `teamgram_delivery_pending{queue}` | 全数据库 pending，包含延迟重试和正在租用的行 |
| `teamgram_delivery_oldest_pending_seconds{queue}` | 最老 pending 年龄，空队列为 0 |
| `teamgram_delivery_expired_leases{queue}` / `max_attempts` | 已过期租约数 / pending 的最大尝试次数 |
| `teamgram_delivery_backlog_sample_timestamp_seconds` / `sample_errors_total` | 最后成功采样时间 / 采样失败次数 |
| `teamgram_delivery_publications_total{queue,outcome}` | 发布结果；timeout 可能是发送结果不确定 |
| `teamgram_delivery_publication_inflight{queue}` | 当前独立投递发布数；受 `DeliveryConcurrency` 有界控制 |
| `teamgram_delivery_claim_wait_seconds{queue}` | durable intent 入队到成功取得租约的等待时间；可定位 worker/数据库 claim 排队 |
| `teamgram_delivery_publication_seconds{queue}` | 等待 Kafka 发布结果的时间；不代表 consumer 或客户端已收到 |
| `teamgram_delivery_completion_seconds{queue,stage}` | 入队到 `consumer_ack` 或 `broker_ack`；不表示端到端接收 |
| `teamgram_sync_delivery_seconds{stage}` / `teamgram_sync_delivery_total{stage,outcome}` | sync consumer、update 准备、在线 session 查询、gateway push、完成标记和整体 fan-out 的阶段耗时/结果；不带用户或消息 ID |
| `gnetway_conn_active{proto}` | 网关当前保持的 TCP/WebSocket/HTTP 连接数；1000 在线验收以保持阶段最低值计分 |
| `teamgram_kafka_producer_outstanding{topic}` | 进程内同 topic 的多个 producer 总占用；64 是单 producer 上限 |
| `teamgram_kafka_producer_calls_total` / `delivery_results_total` | 调用返回结果 / 含迟到 ACK 的最终 broker 结果 |

数据库采样独立于 publisher，每 15 秒一次、单次最多 5 秒。msg 多副本采样同一全局队列，仪表盘用 `max by(queue)`，不要 `sum`；查询成本随 pending 数量增长，生产容量验证时需检查实际查询计划和扫描成本。

规则覆盖老积压、老积压与反复失败同时出现、发布错误/超时、单个实例采样缺失/过旧。最大重试次数与最老年龄可能来自不同行，告警后须查询具体 intent。不要删除 pending 消息来消除告警；先修复 broker/consumer/权限或永久错误，再依靠幂等重放恢复。

## 诊断记录

本轮隔离验收遇到并修复了五类实验配置问题：构建忽略规则遗漏 BFF 的 `config.json`；Web GramJS 使用 WebSocket 却初始映射到原始 TCP 10443（现为容器 11443）；种子永久 auth key 漏写 `perm_auth_key_id=self`；SeaweedFS 4.47 入口只修复 `/data` 所有权并降为 `seaweed` 用户，原 `/storage` 数据卷不可写，`user: 0:0` 也无法修复（现使用 `/data`，由镜像入口处理权限）；`server` 使用 `-master.telemetry=false`，不支持 `-telemetry=false`，错误参数会让隔离对象存储退出。这些修复均不改变公开 TL 方法。

Web 客户端的 TL 方法已更新但默认握手常量仍为 228；探针覆盖初始化层号并校验实际发送构造器，因此没有修改用户正在开发的客户端代码。

Docker Desktop 的 BuildKit DNS 曾无法解析 Ubuntu/Go 下载域名。通过宿主实时 DNS/DoH 解析后为本次 build 传入 `--add-host` 可恢复，参照已有 `docker/production/build-r29.sh`；地址应每次解析，不把临时 IP 当作永久依赖。

## 本地验收记录（2026-10-15）

最终消息回归报告为 `/tmp/teamgram-messaging-reliability-final-20261015.json`，`status` 与 `cleanup.status` 均为 `PASS`；完整验收镜像 ID 为 `sha256:1baf4ddde04e5562b1fd86b8803b5a15d214252d2fc9a5bcbc8fc2082ea3868b`，Compose SHA-256 为 `af09ed02fd6cd2d996047640a8c99792293d505584e0947bb5819cf470afdebd`。早期基线报告仍保留在 `/tmp/teamgram-messaging-reliability-verified-20261015.json`。

- 双向顺序负载共 40 条，完整镜像下 `pts`、历史唯一性、同/新 session 的 `random_id` 重放均通过；backend 替换后进程健康耗时 20.733 秒，连同消息验证共 25.427 秒。
- Kafka 停机约 15.4 秒，SQL 验证 2 条 sender 消息、0 条 recipient 消息、4 条 pending intent；发送后的独立采样仍观测到 4 条积压。协议就绪后，原 session 在读取状态或历史前收到实时 update。故障消息从发送到送达最长约 61.1 秒，此恢复尾延迟仍需生产环境优化与验收。
- PostgreSQL 18 空库应用 96 个迁移；备份 manifest 验证、PITR（1.792 秒）、容器替换恢复（1.782 秒）、隔离旧主后的手工提升（1.996 秒）均通过，演练标记无丢失。
- Go race 与真实 PostgreSQL/Kafka 回归、48 项 Python 回归、严格 TypeScript 检查、`promtool`、迁移可移植性和 Compose 检查通过。结束时 7 个依赖全部存活，配置健康检查的服务均为 healthy，SQL pending 为 0；各隔离实验的容器、网络和数据卷均无残留。SeaweedFS 的 S3 健康检查作为 backend 启动依赖。

## 验证范围

本地报告已增加 5000 条 MTProto 长连接的隔离验收，但仍未证明长期可用性、1000 条消息/秒、频道大群 fan-out、媒体/CDN、跨区域切换、真实注册/短信、支付/provider、生产告警送达或完整 Layer 229 方法兼容性。正式上线前需在目标部署环境定义容量、RPO/RTO 和 SLO，完成持续压测、慢依赖/磁盘耗尽演练、备份异机恢复以及自动 HA 的隔离与仲裁验收。

## 1000 在线连接验收

`tools/messaging_online_capacity_probe.py` 在全新 PostgreSQL 18/Kafka/Redis/etcd/MinIO/Prometheus 栈中生成指定数量的隔离 auth key，使用 GramJS Layer 229 客户端分批建立长连接。探针先用一条连接完成网关和会话预热（瞬时传输错误最多重试两次），然后以 50 条一批建立剩余连接；保持阶段按客户端 `isConnected()` 和 Prometheus `gnetway_conn_active` 的最低值计分，抽样断开并重连 10 条，最后等待活动连接 gauge 回到 0。

```sh
python3 tools/messaging_online_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-online-v1 \
  --users 1000 --hold-seconds 20 --batch-size 50 --reconnect-count 10 \
  --output /tmp/teamgram-online-capacity-1000.json
```

本轮报告为 `/tmp/teamgram-online-capacity-1000-20261015-final.json`，镜像 ID 为 `sha256:5eebef110914f2fd3f73e9943b1bb5cdda02065a65375a09df60b68bac1b8813`：1000/1000 auth 会话建立成功，保持阶段客户端和服务端最低均为 1000，10/10 抽样重连成功，断开后服务端 gauge 为 0，隔离资源清理通过。该结果证明本地单实例在该连接模型下可保持 1000 在线会话；它不等价于生产容量或自动高可用承诺。冷启动时一次未经预热的 50 条并发批次曾全部超时，因此生产部署仍需预热、连接准入和目标环境的冷启动压测。

同一镜像另跑了独立的双账号低频消息档 `/tmp/teamgram-capacity-online-v1-10s-20261015.json`：20 秒计划 200 条，ACK 200/200，实时送达 200/200，ACK p95 18.2ms、实时送达 p95 801.9ms；该档只验证消息路径回归，未与 1000 条连接同时施压。

## 1000 在线加消息负载

在线连接和消息链路使用同一隔离栈同时验收。命令如下：

```sh
python3 tools/messaging_online_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-online-v1 \
  --users 1000 --hold-seconds 20 --batch-size 50 --reconnect-count 10 \
  --timeout-ms 60000 --mix-rate 10 --mix-seconds 20 --mix-max-inflight 32 \
  --output /tmp/teamgram-online-mixed-1000-10s-20261015-final.json
```

本轮报告通过，镜像 ID 为 `sha256:5eebef110914f2fd3f73e9943b1bb5cdda02065a65375a09df60b68bac1b8813`：1000/1000 连接认证并保持，客户端和服务端保持阶段最低均为 1000，10/10 重连成功；连接保持期间计划发送 200 条消息，ACK 和实时送达均为 200/200，无丢弃，`pts` 连续、历史与实时 ID 一致、outbox 排空。ACK p95 为 18.2ms，实时送达 p95 为 506.7ms，资源清理通过。该结果证明本地单实例在 1000 在线连接加 10/s 私聊负载下通过本轮预算，仍不代表生产容量、长期 SLO 或自动高可用。

## 5000 在线连接验收

5000 档使用相同的 Layer 229 会话模型，分 50 条一批建立连接，并抽样重连 20 条：

```sh
python3 tools/messaging_online_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-online-v1 \
  --users 5000 --hold-seconds 20 --batch-size 50 --reconnect-count 20 \
  --timeout-ms 60000 \
  --output /tmp/teamgram-online-capacity-5000-20261015-rerun.json
```

报告 `/tmp/teamgram-online-capacity-5000-20261015-rerun.json` 通过：5000/5000 连接认证，客户端和服务端保持阶段最低均为 5000，20/20 重连成功，活动连接 gauge 从 5000 回到 0；探针 RSS 峰值约 1.38 GiB，PostgreSQL、Kafka、Redis、etcd、MinIO、backend 和 Prometheus 均保持运行，隔离资源清理通过。

同一栈再执行 5000 在线加 10/s 消息负载：

```sh
python3 tools/messaging_online_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-online-v1 \
  --users 5000 --hold-seconds 20 --batch-size 50 --reconnect-count 20 \
  --timeout-ms 60000 --mix-rate 10 --mix-seconds 20 --mix-max-inflight 32 \
  --output /tmp/teamgram-online-mixed-5000-10s-20261015.json
```

早期报告 `/tmp/teamgram-online-mixed-5000-10s-20261015.json` 通过：5000 连接保持、200 条计划消息全部 ACK 和实时送达，实时送达 p95 为 533.1ms。分区拓扑调整后，报告 `/tmp/teamgram-online-mixed-5000-10s-sixpart-r5b-20261015.json` 再次通过：5000/5000 连接保持，20/20 重连，200/200 消息 ACK 与实时送达，ACK p95 为 31.8ms、实时送达 p95 为 86.2ms，最大 outbox pending 为 2，`pts` 连续、历史与实时 ID 一致并排空。两次都是本地单实例合成验收，不能直接作为生产容量或长期 SLO 承诺；频道 fan-out、多副本和跨故障域仍需单独验收。

### 双副本 5000 在线 + 20/s 回归

相同的 5000 在线、双后端各 2500 连接、20/s × 20 秒负载连续运行三轮：

| 轮次 | 镜像 | 最终送达 | ≤2s 实时送达 | 实时 p95 / p99 |
| --- | --- | ---: | ---: | ---: |
| r6 | `teamgram-reliability:20261015-dual-backend-commit-metrics-r6` | 400/400 | 384/400 | 1,328ms / 6,868ms |
| r7 | `teamgram-reliability:20261015-dual-backend-commit-metrics-r7` | 400/400 | 386/400 | 1,569ms / 6,371ms |
| r8 | `teamgram-reliability:20261015-dual-backend-safe-commit-batch-r8` | 400/400 | 400/400 | 500ms / 756ms |

r6 报告为 `/tmp/teamgram-online-dual-5000-20s-20261015-r6.json`，r7 报告为 `/tmp/teamgram-online-dual-5000-20s-20261015-r7.json`，r8 报告为 `/tmp/teamgram-online-dual-5000-20s-20261015-r8.json`。三轮均通过 5000 会话保持、2500/2500 副本分布、20/20 抽样重连和 400/400 最终送达；r6、r7 因部分实时更新超过 2 秒而未通过消息时延预算。

r7 的分区指标显示 `Sync-T` consumer 每条记录同步提交 offset：810 次提交的平均耗时约 61ms，分区 p95 桶分别为 128ms 和 512ms；记录进入 handler 时的 p95 桶为 2.048s。r8 仅对有 Kafka offset/request hash 持久化 receipt、且重放复用已准备 seq/date 的 `Sync-T` `TLSyncPushUpdates` 启用有界 offset 合批：每分区最多 8 条或 250ms 提交一次。其他 Sync 方法、`BotFather-T` 和 Inbox 仍逐条同步提交；rebalance 取消时未提交的批次保持未确认。r8 提交数降为 513 次，平均约 48ms、两个分区 p95 桶均为 128ms；record age p95 桶降至 16ms。测试覆盖安全消息合批、遇到非安全消息时提交已处理前缀，以及窗口到期提交。

本地 r8 命令：

```sh
python3 tools/messaging_online_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-dual-backend-safe-commit-batch-r8 \
  --users 5000 --backend-instances 2 --hold-seconds 20 --batch-size 50 \
  --reconnect-count 20 --timeout-ms 60000 --mix-rate 20 --mix-seconds 20 \
  --mix-max-inflight 256 \
  --output /tmp/teamgram-online-dual-5000-20s-20261015-r8.json
```

r8 通过全部 400 条消息的 1s ACK、2s 实时送达预算，`pts` 连续、历史与实时 ID 一致、outbox 排空且隔离资源清理通过。该结果是单 Kafka broker、单 PostgreSQL、两个合成私聊用户和本地双后端的短时验收；它不证明生产容量、Kafka 多副本耐久性、长期 SLO、群组 fan-out 或自动跨故障域 HA。

### 5000 人频道与超级群组 fan-out

`tools/messaging_group_channel_fanout_probe.py` 在新的隔离环境创建一个广播频道和一个 megagroup，为每个目标加入 5000 个合成用户；5000 个 Layer 229 客户端保持实时更新订阅，双后端各承载 2500 个客户端，然后分别向两个目标发布一条消息。消息必须由每个客户端实时收到，且目标客户端可读取历史；采样 ACK 预算为 1 秒，计划发送到实时更新的预算为 2 秒。基础 group-chat 在 Telegram 语义中有成员上限，本档以可容纳 5000 成员的超级群组（megagroup）作为群组广播验收对象。

复跑报告 `/tmp/teamgram-group-channel-fanout-5000-20261015-rerun.json`：5000/5000 客户端认证、订阅并保持在线，两个后端各 2500；频道与超级群组分别 5000/5000 唯一收件、零重复、历史可读、频道 `pts` 有效，两个投递 outbox 最终均完成，连接清理和隔离资源清理通过。但该负载未通过时延预算：频道 ACK 5,029ms、实时 p95/p99 19,575/20,283ms；超级群组 ACK 5,008ms、实时 p95/p99 18,444/19,119ms。2 秒内收到的收件者分别为 333/5000 和 660/5000。报告判定 `BUDGET_EXCEEDED`，不是完整送达失败。

下一轮复测 `/tmp/teamgram-group-channel-fanout-5000-20261015.json` 也观测到两类目标完整送达且零重复，实时 p95 分别为 15,086ms 和 16,265ms；首次运行的连接清理 gauge 判定有探针错误，因此以修正 gauge 后的复跑作为验收结论。复跑客户端进程 RSS 约 1.43GiB、事件循环 p95 约 22ms；PostgreSQL 观测连接为 156/300。该探针只发一条频道消息和一条超级群组消息，是本地短时容量样本，不等于群组持续消息吞吐、频道长期负载或生产容量。

实现路径包含两个成本段：频道消息事务同步写入成员 recipient 快照后才返回；异步投递再做资格检查、调用 Sync 并持久化完成状态。对应实现位于 `app/bff/apifull/internal/domain/channel_delivery_outbox.go` 和 `app/bff/apifull/channelview/delivery.go`。资格批查与有界 fan-out 已实现，但不能单凭路径推断它们是当前实时尾延迟根因；下面的分阶段计时显示，5000 人档中 APIFull 到 Sync 的 RPC 大多很快返回，而 Sync Kafka handler 开始处理前已有显著排队年龄。

批量资格查询后的 100 人回归报告 `/tmp/teamgram-fanout-100-channel-batch-20261015-r3.json` 仍为 `BUDGET_EXCEEDED`。两后端各保持 50 条会话；频道与超级群组均完成 100/100 唯一收件（98 条在线实时收件，2 条断线后通过差量恢复），无重复、`pts` 连续、历史可读、outbox 排空。ACK 为 89.7/66.0ms，离线恢复 p95 为 27.7/24.5ms；但实时 p95 为 5,672.8/5,627.6ms，2 秒内分别只有 49/98 和 51/98 条实时收件。与前序 100 人基线约 5.16-5.18 秒相比没有收益，因此没有把这版外推到 5000 人验收。探针侧 Node event-loop p95 为 21.7ms、p99 为 24.6ms，PostgreSQL 连接为 145/300；现有报告没有 APIFull 收件资格查询、Sync RPC 推送、网关写出和回执持久化的分阶段耗时，不能据此归因尾延迟。下一步应补充这些阶段的计时/追踪，再按同一负载定位瓶颈，不继续盲目调高并发或数据库连接数。

在该 100 人复测基础上加入 APIFull 分阶段观测后的报告 `/tmp/teamgram-fanout-100-channel-stage-r4-20261016.json` 仍未通过 2 秒实时预算：频道实时 p95/p99 为 5,678/6,180ms，超级群组为 4,638/5,641ms；ACK 为 106/93ms，两类目标各 98/98 在线收件，完整送达、恢复、PTS、outbox 和资源清理均通过。两目标资格查询总耗时约 4.3ms、回执持久化约 15.0ms，APIFull 到 Sync 的 RPC 等待总计约 952ms（202 次调用）；Sync `consumer` handler 总计约 4.09s，gateway 写队列等待总计约 653ms。该报告只有阶段计数和总和，不能由聚合值推断 p95，也未覆盖 Kafka record age。

R5 报告 `/tmp/teamgram-fanout-100-channel-stage-r5-20261016.json` 增加直方图后，频道与超级群组实时 p95/p99 分别为 5,172/6,184ms 和 5,641/7,157ms，98/98 在线收件最终完整，无重复；ACK 为 105/79ms。资格查询、回执持久化均远低于毫秒级累计成本，Sync RPC 的 99% 以上调用在 25ms 内返回。`mqconsumer_record_age_seconds` 的 95 分位落在 4.096-8.192 秒桶，Sync handler 的 95 分位不超过 25ms，offset commit 的 95 分位落在 256-512ms 桶。这把主要延迟缩小到 Kafka 记录进入 handler 之前的等待和提交，但当时仍是 100 人小档。

将有界 offset 合批从已带持久化回执的 `TLSyncPushUpdates` 扩展到 `TLSyncPushUpdatesIfNot` 后，增加了 PostgreSQL 回放测试：相同 offset 重放不重复增加 `seq`，也不重复查询在线会话。6 分区 r6 的 100 人报告 `/tmp/teamgram-fanout-100-channel-stage-r6-20261016.json` 显著改善为频道实时 p95 1,613ms、超级群组 1,102ms，ACK 93/86ms；最终完整送达、无重复、PTS 连续、恢复和清理均通过。但频道只有 96/98 在线收件在 2 秒内完成，未满足“全员 2 秒”门槛，所以仍是 `BUDGET_EXCEEDED`。这一轮说明安全 offset 合批有收益，不表示 5000 人已经通过。

同一 r6 镜像（`sha256:d3d8a25934b56ac64e309882dc44b4bc7e0c910ac5afe8da1abe13e792a24b14`）的 5000 人、双后端、每目标 100 人离线恢复报告 `/tmp/teamgram-group-channel-fanout-5000-20261016-r6.json`：频道和超级群组各有 4900/4900 在线收件、100/100 差量恢复、零重复、PTS 连续且 outbox 排空；但 ACK 为 1,903/2,089ms，实时 p95/p99 为频道 30,933/37,354ms、超级群组 27,773/32,281ms。离线恢复 p95 为 246/324ms，资源清理通过。Sync Kafka record age 的全局样本 p95 位于 16.384-32.768 秒边界之间，offset commit 的全局样本 p95 位于 256-512ms 之间；Sync handler 大多数不超过 25-50ms。与此同时，APIFull `sync_rpc` 直方图中 99% 以上调用不超过 25ms。因此该档的主要积压发生在 broker 接受记录之后、Sync handler 持续消费之前，而非资格 SQL 或单次 gateway push。

分区数对照没有找到可接受的加分区解法。48 分区报告 `/tmp/teamgram-group-channel-fanout-5000-20261016-r7-48part.json` 判定 `FAIL`：频道有 6 个重复更新，超级群组在线实时收件少 64 个并有 6 个重复；实时 p95 为 12,881/80,031ms，ACK 为 5,041/5,088ms。record age 的 95 分位高于 32.768 秒桶，两个 backend 在负载期间约 100% CPU。12 分区报告 `/tmp/teamgram-group-channel-fanout-5000-20261016-r8-12part.json` 也判定 `FAIL`：频道/超级群组在线实时缺少 42/47 个收件者，虽无重复，实时 p95 仍为 32,132/28,726ms，ACK 为 3,061/3,082ms；离线 cohort 最终可恢复，outbox 和资源清理通过。其 record age 的 95 分位仍在 16.384-32.768 秒桶。48 分区时两个消费者组在每个分区仍只能各有一个 owner，扩大分区也同时增加调度与服务 CPU 开销；该本地实验不构成生产 topic 扩分区建议。

目前的 5000 人档均未通过时延验收；r7/r8 还出现实时重复或缺收，必须作为硬失败处理，即使离线 difference 最终能够补齐。随后使用完整源码镜像、同一 5000 人负载和 6 分区完成 `sync MaxConns: 8` 与 `16` 的各两轮 A/B：四轮都保持唯一收件、PTS、历史、恢复和 outbox 正确，16 连接的 `prepare_if_not`/`mark_complete_if_not` 聚合耗时两轮都较低，但 Kafka record age 和 handler queue 桶没有稳定变化，实时 P95 仍为约 3.5–4.7 秒。当前保留 16 作为本地测量候选值，不能把它解释为生产容量或 2 秒实时达标；后续仍需追踪每条回执从入队、处理、gateway ack 到客户端观察的对应关系，并在固定资源的环境中回归。不要再仅凭分区数或 worker 并发调整宣称容量改善。

### 频道更新缺口恢复验收

`tools/messaging_channel_recovery_probe.py` 使用两个 Layer 229 账号创建隔离频道：成员 B 读取基准 `pts` 后断开，A 连续发布 5 条消息，B 重连并从旧 `pts` 调用 `updates.getChannelDifference`，页大小为 2。复测报告 `/tmp/teamgram-channel-difference-recovery-20261015-offline-gap.json` 为 `PASS`：B 在离线期间错过 5/5 条实时更新，随后按 `pts=2/4/5`、2/2/1 条恢复全部内容，最终页为 `final=true`；追平后返回 `updates.channelDifferenceEmpty`，频道历史与恢复内容一致。非成员返回 `USER_NOT_PARTICIPANT`，错误 access hash 返回 `CHANNEL_INVALID`；临时频道数据库行和隔离容器、网络、卷均清理为 0。

```sh
python3 tools/messaging_channel_recovery_probe.py --execute \
  --image teamgram-reliability:20261015-channel-recovery \
  --probe-timeout 60 \
  --output /tmp/teamgram-channel-difference-recovery-20261015-offline-gap.json
```

这是单频道、单成员的本地协议与离线缺口验收，不测 5000 人 fan-out 的时延/容量，也不覆盖网关进程故障、TCP 分区或生产 SLO。后续仍需把相同缺口恢复检查并入 5000 成员压测及传输层故障演练。

### Telegram 官方公开语义

Telegram 没有公开服务端代码，因此不能据此断言其内部队列、数据库或 fan-out 实现。公开协议规定频道和超级群组各有独立的 `pts` 更新序列；客户端用 `pts`/`pts_count` 去重并检测缺口，再通过 `updates.getChannelDifference` 分页补齐。超级群组在协议中也是带 `megagroup` 标志的 channel。当前项目已把频道事件与消息/`pts` 在同一事务中持久化，并实现该差量接口；本地离线探针已验证旧 `pts` 分页恢复及成员权限边界。该证据不代表大规模 fan-out 或生产故障恢复。参考 [Working with Updates](https://core.telegram.org/api/updates)、[updates.getChannelDifference](https://core.telegram.org/method/updates.getChannelDifference)、[Channels and supergroups](https://core.telegram.org/api/channel) 与 [Telegram FAQ](https://telegram.org/faq)。

公开浏览与成员订阅是不同权限。Telegram FAQ 说明公开群组可供任何人查看历史；API 邀请文档还定义 `chatInvitePeek`，允许未加入用户在预览有效期内通过 `updates`、`messages.getHistory` 和 `channels.getMessages` 读取超级群组或频道内容。参考 [Telegram FAQ: Public groups](https://telegram.org/faq#q-what-39s-the-difference-between-groups-and-channels)、[Invite links: Public usernames and previews](https://core.telegram.org/api/invites)。APIFull 现在允许持有有效频道 access hash 的用户读取公开用户名频道/超级群组的历史、按 ID 取消息和搜索结果；返回的 `Chat.left` 表示尚未加入。私有频道仍返回 `CHANNEL_PRIVATE`，活跃的 view-messages 封禁仍拒绝读取；发送、转发、回复引用、成员管理、消息浏览计数和已读状态继续要求成员资格。私有超级群组/频道通过 `messages.checkChatInvite` 成功检查后返回 `chatInvitePeek`，授权写入 PostgreSQL 并在 1 小时或邀请自身更早过期时失效；有效期内的 `messages.getHistory`、`channels.getMessages`、搜索和 `updates.getChannelDifference` 均可读，过期后恢复原有权限错误。本节 fan-out 探针仍只覆盖已加入成员的实时推送。

复跑命令：

```sh
python3 tools/messaging_group_channel_fanout_probe.py --execute \
  --image teamgram-reliability:20261016-channel-stage-r6 \
  --users 5000 --backend-instances 2 --sync-partitions 48 --batch-size 50 \
  --timeout-ms 60000 --delivery-timeout-seconds 180 \
  --output /tmp/teamgram-group-channel-fanout-5000-20261016-r7-48part.json
```

## 固定速率容量验收

`tools/messaging_capacity_probe.py` 使用相同的全新隔离栈，按绝对计划时间交替双向发送。默认每档 60 秒、5/10/20 条每秒、最多 32 个等待 ACK/实时送达的调用；隔离 Kafka topic 使用 6 个分区。种子用户 `910001` 与 `910002` 在 3 分区配置下会落到同一分区，使双向消息被一个 consumer 串行处理；6 分区配置将它们分开。该调整只校准本地双账号压测拓扑，不能换算成多用户、群组或生产最大容量。

```sh
python3 tools/messaging_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-acceptance \
  --rates 5 10 20 --seconds 60 \
  --output /tmp/teamgram-capacity-acceptance.json
```

完整验收镜像的本地结果写入 `/tmp/teamgram-capacity-acceptance-20261015.json`。每档 60 秒、最多 32 个在途调用：

| 计划速率 | 计划消息 | ACK 成功 | 实时送达成功 | ACK ≤1s | 实时送达 ≤2s | 结果 |
| ---: | ---: | ---: | ---: | ---: | ---: | --- |
| 5/s | 300 | 300 | 300 | 300 | 300 | PASS |
| 10/s | 600 | 600 | 600 | 600 | 600 | PASS |
| 20/s | 1,200 | 900 | 900 | 900 | 509 | BUDGET_EXCEEDED |

早期完整镜像的 20/s 档有 300 个压测端 `MAX_INFLIGHT` 丢弃槽位，已观察实时送达 p95 约 2.646 秒；这证明当时的压测配置超出预算，不能直接推导服务端容量上限或数据丢失。报告 `highestPassingTestedRate=10`，`capacityCeilingFound=false`。

独立数据库积压采样分别记录 27、27、28 个样本，观测到的最大 pending 为 3、7、64；每档排空后的 SQL pending 都为 0，清理屏障和资源清理均通过。

早期三分区拓扑的 20/s 报告 `/tmp/teamgram-capacity-20s-20sec-queuedgateway-r4-20261015.json` 为 `PROFILE_FAILED`：400/400 ACK、364/400 实时送达、实时 p95 约 7.41 秒、pending 峰值 236，且检查到 `pts` 间隔。种子用户 `910001` 与 `910002` 的 Kafka hash 都落在分区 2；两个方向因此由同一个 Inbox consumer 串行处理。阶段指标印证积压发生在前段：outbox claim 等待累计约 3007 秒/701 次，而 gateway 写队列等待累计约 32ms、无拒绝；sync gateway push 累计约 0.59 秒。

固定速率探针改用 6 分区并合并网关 WebSocket 批量写出后，镜像 `teamgram-reliability:20261015-stage-r5b`（`sha256:938f54f39b801bd27fc325d5eb89442d31abae47edfdffdbdd734bc515028a15`）以 20/s、20 秒、`max-inflight=256` 复测通过，报告 `/tmp/teamgram-capacity-20s-20sec-sixpart-r5b-20261015.json`：400/400 ACK 和实时送达，全部在 1 秒/2 秒预算内，实时 p95 334ms、p99 631ms，无生成器丢弃，`pts`/历史一致，pending 峰值 2、最终为 0。claim 等待累计约 38 秒/730 次，gateway 写队列等待累计约 34ms，无拒绝。10/s 回归报告 `/tmp/teamgram-capacity-10s-20sec-sixpart-r5b-20261015.json` 也是 200/200，实时 p95 143ms。

加入网关写失败计数和失败时关闭连接的 r5c 镜像 `teamgram-reliability:20261015-stage-r5c`（`sha256:61ea91e4a02b668e7dc5e9206f0f2c6326f4a7205bf94190a9d2c4222669f881`）再次按 20/s、20 秒、`max-inflight=256` 验收：

```sh
python3 tools/messaging_capacity_probe.py --execute \
  --image teamgram-reliability:20261015-stage-r5c \
  --rates 20 --seconds 20 --max-inflight 256 \
  --output /tmp/teamgram-capacity-20s-20sec-sixpart-r5c-20261015.json
```

报告 `/tmp/teamgram-capacity-20s-20sec-sixpart-r5c-20261015.json` 为 `PASS`：400/400 ACK、400/400 实时送达，400 条均分别在 1 秒和 2 秒预算内，无生成器丢弃；计划时刻到实时送达 p95 为 272ms、p99 为 582ms，ACK p95 为 16.5ms；`pts` 与历史一致，采样 pending 峰值 2、排空后为 0，资源清理通过。claim 等待累计约 34.6 秒/800 次，gateway 写队列等待累计约 36.5ms/854 次，本窗口未观测到写失败或队列拒绝。

该结果说明此前 7.41 秒尾延迟的直接原因是两个固定账号在三分区下落到同一个 Inbox 分区、导致双向消费串行；改用能分散这两个 key 的隔离拓扑后，本地实测 p95 降至 272ms，400/400 实时送达。它不能据此归因或排除生产 PostgreSQL、Kafka broker 等其他负载问题，也不代表已找到生产容量上限。生产环境需要核实 `Inbox-T`、`Sync-T` 的实际分区数、key 分布、每分区 consumer lag 和副本数；按真实消息速率与故障恢复预算做容量规划，同时保持同一用户更新的分区顺序。隔离验收的 6 分区只是为这两个固定 key 解开碰撞，不是生产推荐值。

在较新的 fan-out r10 镜像上复跑 60 秒 20/s，报告 `/tmp/teamgram-capacity-20s-telegram-learn-r12-20261015.json` 为 `PASS`：1200/1200 ACK、1200/1200 实时送达、零生成器丢弃；计划时刻到 ACK p95/p99 为 16.9/87.7ms，实时送达 p95/p99 为 114.3/406.5ms；`pts` 连续、历史与实时一致、outbox 排空、隔离资源清理通过。gateway 写队列未观测到拒绝或写失败。该结果仅覆盖本地双账号私聊链路，不能解释或替代 5000 成员频道/超级群组的 APIFull fan-out 验收，也不代表生产容量。

网关批量编码或 socket 写出失败时会记录 `gnetway_gateway_write_failed_total{reason}` 并关闭连接，避免静默丢弃已出队更新；客户端重连后的 `pts` difference 是恢复路径。`TestFlushGatewayWritesClosesConnectionOnSocketWriteFailure` 使用已关闭对端的 `net.Pipe` 分别注入 TCP 和 WebSocket 写失败，并验证 gateway 关闭连接；这验证了本地写错误处理分支，不等同于容器网络切断、客户端自动重连和 MTProto difference 的端到端故障演练。

每个计划槽都保留在分母。压测端迟到超过 100ms、并发占满、RPC 错误和送达超时均保留失败；不降低速率补发。报告区分计划到 ACK/送达、实际发出到 ACK/送达、窗口内 goodput 与窗口后排空。百分位明确只来自已观察结果，验收同时要求全部计划消息在 ACK 1 秒、在线送达 2 秒内完成。未达到预算或一致性检查失败不会标成已找到系统容量上限。

固定速率探针还保存每个档位的 Prometheus 阶段计数快照和按有限标签计算的 delta：outbox claim、Kafka publication、consumer ACK、sync fan-out、gateway push 及结果分类。快照是诊断证据，不会在指标暂时不可抓取时把容量结果改判为通过；容量结论仍由消息 ACK、实时送达、`pts` 连续性和 outbox 清空决定。

负载期间不逐条读状态或历史；结束后合并 outgoing ACK 与 incoming live 的 `pts` 区间，要求连续且双方最终状态一致，历史和实时 message ID 唯一。runner 以 SQL outbox 零积压作为清理屏障，再删除 tag、读回零剩余，并再次排空清理生成的状态意图。历史按服务当前 50 条上限分页，报告输出等待 stdout 写完，防止较大 JSON 被进程退出截断。

## 30 天 SLO 证据

`tools/messaging_slo_observer.py` 在新建本地栈中每个 UTC 分钟执行双向各一条 canary，使用与容量探针相同的实际计划时间、ACK 和实时事件。每桶结算后写入并 fsync JSONL，同时保留原始探针和 SHA-256。观察文件必须是新路径；进程中断、重启后空缺的分钟保留为空缺。该工具不安装定时任务，也不发送外部通知。

```sh
python3 tools/messaging_slo_observer.py --execute \
  --image teamgram-reliability:20261015-acceptance --minutes 10 \
  --evidence /tmp/teamgram-slo-new-observation.jsonl \
  --output /tmp/teamgram-slo-observer.json
python3 tools/messaging_slo_window.py /tmp/teamgram-slo-new-observation.jsonl \
  --output /tmp/teamgram-slo-30day.json
```

聚合窗口严格为 `[end-30天,end)` 的 43,200 个 UTC 分钟，默认目标 99.95% 只是本仓库候选预算。ACK/在线送达分别按 `sum(good)/sum(eligible)`，不平均 p95；canary 每分钟计划两条，漏发不剔除，重试不能覆盖原失败。重复/冲突桶、重叠、错误 scope/Layer、未来证据为 `INVALID`；缺分钟、未结算、collector 不完整或 counter reset 为 `INCOMPLETE`；完整无业务流量为 `NO_TRAFFIC`。已观测错误超过固定 canary 全窗口预算时，即使缺证据也为 `FAIL`。普通业务流量必须单独以 `organic-layer229` 提供逻辑消息证据，且满足明确的活跃分钟下限，不能用本地合成探测替代。

本轮实际完成两分钟、四次真实合成探测。最新报告 `/tmp/teamgram-messaging-30day-slo-final-20261015.json` 为 `INCOMPLETE`：已观察 2/43,200 桶，缺失 43,198 桶；ACK 和实时探测均为 4/4。ACK/实时探测成功不能据此声称长期 SLO 达成；`productionSloAccepted` 保持 false。

## PostgreSQL 自动 HA 实验

独立 HA Compose 使用 PostgreSQL 18、Patroni 4.1.5、三节点 etcd 和一个 HAProxy。同步复制启用 strict 模式、一个同步备库，关闭 DCS failsafe；代理只路由 `/primary` 返回 200 的节点。本配置只用于本地隔离演练，DCS 与 Patroni REST 未配置生产 TLS/访问控制。

```sh
docker build --pull=false -t teamgram-patroni:4.1.5-pg18 docker/reliability/ha
python3 tools/postgres_ha_probe.py --execute \
  --output /tmp/teamgram-postgres-ha.json
```

探针必须先确认实际同步备库已登记到 DCS `/sync`，再以 `synchronous_commit=remote_apply` 提交标记并立即注入故障。仅查看 `pg_stat_replication.sync_state` 不足以证明该备库已经具备自动提升资格；早期演练因此超时，失败记录保留。故障标记之后没有等待异步备库追上的人工屏障。

断言覆盖主节点整个容器 SIGKILL、旧主网络隔离、旧主恢复为备库、etcd 丢失多数派及恢复。直接在旧主 Unix socket 查询 `pg_is_in_recovery=true`，并明确要求 INSERT 返回 `25006`，认证/缺表等错误不得算隔离成功。每阶段比较完整标记 ID 和 payload，恢复后必须再次成功提交；记录镜像 ID、配置哈希和失败日志。时间包含 Docker 注入、轮询、代理恢复、读回及提交，是本地观测的写恢复耗时。

严格版两次报告 `/tmp/teamgram-postgres-ha-registered-20261015.json` 与 `/tmp/teamgram-postgres-ha-repeat-20261015.json` 均通过并清理无残留：崩溃写恢复 30.475/29.008 秒，网络隔离后 27.293/27.809 秒，失去多数派后拒写 18.192/13.948 秒；已确认标记内容无丢失，旧主恢复为备库。初始主分别为 pg1/pg2。

共享 Docker Desktop VM 未启用 watchdog 或 STONITH，不能证明 Patroni 冻结而 PostgreSQL 存活时的硬隔离。同步主备同时故障、异机/跨区域切换、代理自身故障及完整通信栈 HA 均未覆盖；当前 backend、Kafka、Redis 和对象存储仍是单实例。`productionHaAccepted` 与 `hardFencingTested` 保持 false，生产上线必须使用独立故障域和完整栈的目标配置重新验收。
