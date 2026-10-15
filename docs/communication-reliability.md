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
| `teamgram_delivery_completion_seconds{queue,stage}` | 入队到 `consumer_ack` 或 `broker_ack`；不表示端到端接收 |
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

报告 `/tmp/teamgram-online-mixed-5000-10s-20261015.json` 通过：连接保持阶段服务端 gauge 为 5000，200 条计划消息全部 ACK 和实时送达，无压测端丢弃；ACK p95 为 25.5ms，实时送达 p95 为 533.1ms，`pts` 连续、历史与实时 ID 一致、outbox 排空。以上仍是本地单实例结果，不能直接作为生产容量或长期 SLO 承诺；20/s、频道 fan-out、多副本和跨故障域仍需单独验收。

## 固定速率容量验收

`tools/messaging_capacity_probe.py` 使用相同的全新隔离栈，按绝对计划时间交替双向发送。默认每档 60 秒、5/10/20 条每秒、最多 32 个等待 ACK/实时送达的调用；每档使用同一对账号的私聊，不能换算成多用户、群组或生产最大容量。

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

完整镜像的 20/s 档有 300 个压测端 `MAX_INFLIGHT` 丢弃槽位，已观察实时送达 p95 约 2.646 秒；这证明该压测配置在该负载下超出预算，不能直接推导服务端容量上限或数据丢失。报告 `highestPassingTestedRate=10`，`capacityCeilingFound=false`。

独立数据库积压采样分别记录 27、27、28 个样本，观测到的最大 pending 为 3、7、64；每档排空后的 SQL pending 都为 0，清理屏障和资源清理均通过。

为区分压测端限制，另以 `max-inflight=128` 重跑 20/s，报告 `/tmp/teamgram-capacity-20s-inflight128-20261015.json` 为 `PROFILE_FAILED`：145 个 `MAX_INFLIGHT`、1,055 个 ACK、620 个实时事件，实时送达 p95 约 7.868 秒，pending 峰值约 317，并触发 `pts` 间隔/重复一致性检查。该结果说明扩大客户端并发会暴露实时投递队列瓶颈，应先按发布、消费者确认、sync update 和 gateway 写出阶段定位，再决定 worker、Kafka 分区和服务副本的扩容方式。

每个计划槽都保留在分母。压测端迟到超过 100ms、并发占满、RPC 错误和送达超时均保留失败；不降低速率补发。报告区分计划到 ACK/送达、实际发出到 ACK/送达、窗口内 goodput 与窗口后排空。百分位明确只来自已观察结果，验收同时要求全部计划消息在 ACK 1 秒、在线送达 2 秒内完成。未达到预算或一致性检查失败不会标成已找到系统容量上限。

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
