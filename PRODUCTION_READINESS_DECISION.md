# Teamgram-Server 生产就绪决策报告

**验收日期**: 2026-10-20
**验收方式**: 严格 Layer 229 目录校验 + 隔离 Web E2E + 现有探针证据审查
**验收主体**: Codex 续验
**项目**: Teamgram-Server (PostgreSQL 18 + Telegram Layer 229)

---

## 执行摘要

⚠️ **生产就绪判定**: **条件通过；Layer 229 目录完整，但完整生产验收仍为 0/813**

严格报告确认 `LAYER229_METHOD_LEDGER.csv` 与 Layer 229 方法目录逐项匹配：813/813 方法、构造器 ID 无缺失或重复。这只是协议目录完整性，不代表每个方法已经在真实生产依赖、真实流量和故障恢复条件下验收通过。当前隔离 Web 验收为 21/21 passed（JUnit：`layer229-full-20261020-rerun.xml`），仍只能作为隔离链路证据。

生产验收状态按方法统计为：`PARTIAL_ISOLATED_SESSION_E2E` 433、`PARTIAL_ISOLATED_DB_ONLY` 216、`PARTIAL_COMPONENT_ONLY` 145、`ISOLATED_COMPONENT_ONLY` 6、`ISOLATED_DB_ONLY` 2、`TRANSPORT_ONLY` 11；完整生产通过数为 0/813。关键业务矩阵的 77 PASS、8 FAIL、5 MANUAL 是 90 条链路探针的结果，不是 813 个方法的生产验收计数；即使某条链路探针通过，也不会自动把其中涉及的方法登记为生产通过。仓库已有自有 provider 接口、客户端和隔离 fixture；当前缺的是把权威服务接入目标生产环境，并补齐生产数据、故障恢复、跨设备 fan-out、真实流量和长期 SLO 证据，因此不能声明 `813/813 COMPLETE`。归属分类及逐项证据边界见[Provider 归属与验收状态](docs/provider-ownership-and-readiness-zh.md)。

Provider 状态需按边界区分：独立 Product Identity 服务已存在；Teamgram 的 `TEAMGRAM_AUTH_PROVIDER_*` 则验证 Bot/Web 凭据，不是 Identity JWT/账号绑定接口。Files BFF 已有签名 CDN provider，DFS 使用 S3 存储契约。当前验收所用 fixture 证明隔离契约，不等价于自有权威服务已接入生产并完成恢复验收。Apple/Google 登录、应用商店结算等第三方依赖只在相应产品能力启用时单独验收，不是所有 provider 门槛都必须采购第三方服务。各能力自有边界、外部上游和现有 staging/fixture 证据见[Provider 归属与验收状态](docs/provider-ownership-and-readiness-zh.md)。

### 2026-10-20 续验结果

- `backend-backend-1` 已切换到 `teamgram-server-e2e-stars-gift:20261020-scheduled-fix` 并通过健康检查；PostgreSQL 18.6 运行库登记到 `117_apifull_sponsored_message_context`，新增 `message_id` schema 已预检。
- `core.real-chain` 在新镜像上完成 scheduled message 的发送、查询、取消、到期投递和历史读回，另含私聊、反应、投票、群组/论坛、频道 history/difference；`readonly.constructor-surface` 与 `provider.stars-gifts-catalog` 同轮重跑均为 `PASS`。
- 全量迁移脚本曾在历史 091/096/108/109 checksum 漂移处停止；现在只对这四个精确旧 journal 值增加 schema 完整性例外，完整目录重放已返回 `0`，且没有改写历史 journal。发布前仍需在干净 staging 数据库复核迁移 provenance；权威 provider 的目标环境接入、长期流量和跨设备恢复仍是独立生产门禁。
- 当前统一矩阵快照为 77 PASS、8 FAIL、5 MANUAL。另有 7 条隔离 PostgreSQL/WebSocket 探针本轮重跑通过，其中 5 个方法从 DAO/组件证据提升为隔离会话证据；严格 Layer 229 结果仍为 813/813 source match、完整生产验收 0/813。

---

## 一、验收结论

### 1.1 功能正确性（隔离证据范围）

以下条目代表组件或隔离环境中已观察到的行为，不等价于完整生产验收。

**认证流程**
- ✅ auth.sendCode（SMS/Email 挑战）
- ✅ auth.signUp（新用户注册）
- ✅ auth.signIn（用户登录）
- ✅ auth.resetAuthorizations（会话撤销）
- ✅ Passkey WebAuthn（指纹/人脸登录）

**消息投递**
- ✅ messages.sendMessage（单消息）
- ✅ messages.sendMediaGroup（媒体组）
- ✅ messages.editMessage（消息编辑）
- ✅ 事务保障：canonical message + outbox 表
- ✅ 重试机制：outbox reconciler 定期扫描
- ✅ 幂等性：request_hash 防重复

**多设备同步**
- ✅ 同一用户多会话在线
- ✅ 实时消息推送（异步投递）
- ✅ 已读状态同步
- ✅ 更新状态追踪（pts/qts/seq）

**持久化**
- ✅ PostgreSQL 18 事务
- ✅ 迁移 099-101 覆盖投递终止态、重放审计、访问控制
- ✅ 隔离重启探针中观察到消息不丢失（不覆盖 813 个方法）
- ✅ Outbox 投递完成确认

### 1.2 协议目录与生产验收状态

**Layer 229 方法目录匹配**

| 项目 | 数量 | 结论 |
|------|------:|------|
| 方法目录与 ledger 匹配 | 813/813 | ✅ PASS（目录完整） |
| 完整生产验收 | 0/813 | ⚠️ 未完成 |

**三个数字分别表示什么**

| 数字 | 统计对象 | 结果 | 是否计入完整生产验收 |
|---|---|---:|---|
| 协议目录 | Layer 229 API 方法和 constructor ID | 813/813 匹配 | 否；只证明目录没有漏项或冲突 |
| 关键业务矩阵 | 90 条消息、频道、设置、Bot、支付等链路探针 | 77 PASS、8 FAIL、5 MANUAL | 否；PASS 证明该探针通过，不会自动改写逐方法台账 |
| 阶段性方法证据 | 813 行台账中的隔离会话、数据库或组件证据 | 433 + 216 + 145 + 6 + 2 + 11 = 813 | 否；各行仍标为 partial、isolated 或 transport only |
| 完整生产验收 | 台账中状态为 `PRODUCTION_ACCEPTED`、`ACCEPTED` 或 `PASS` 的方法 | 0/813 | 是；当前没有方法满足并登记完整生产门槛 |

所以 `77 PASS` 与 `0/813` 并不矛盾：前者按业务链路探针计数，后者逐个 API 方法计数；矩阵的 PASS 也不代表已覆盖真实 provider、重启恢复、跨设备 update 和生产流量等全部门槛。

**严格生产验收分层统计**

| production_acceptance_status | 方法数 | 含义 |
|---|---:|---|
| `PARTIAL_ISOLATED_SESSION_E2E` | 433 | 隔离会话链路有证据，生产依赖或恢复证据仍缺失 |
| `PARTIAL_ISOLATED_DB_ONLY` | 216 | 隔离 PostgreSQL/DAO 证据，缺少完整会话或生产证据 |
| `PARTIAL_COMPONENT_ONLY` | 145 | 组件/单元证据，缺少隔离会话和生产证据 |
| `ISOLATED_COMPONENT_ONLY` | 6 | 仅隔离组件证据 |
| `ISOLATED_DB_ONLY` | 2 | 仅隔离数据库证据 |
| `TRANSPORT_ONLY` | 11 | 仅传输包装层，非业务方法验收 |
| **合计** | **813** | **完整生产通过：0/813** |

目录匹配和隔离测试通过都不能直接升级为生产通过；每个方法仍需真实依赖、会话更新、重启恢复和流量证据。

**关键协议检查**
- ✅ RPC 构造器 ID 与 Telegram 标准一致
- ✅ 错误响应格式遵循 RPC 标准（error_code + error_message）
- ✅ 方法参数序列化正确
- ✅ 返回值构造器有效

### 1.3 Bot API 兼容性 ✅ PARTIAL

**路由与 adapter 覆盖**: 185/185 官方方法可被规范化路由接受并进入具体 dispatch；另保留旧客户端使用的 `kickChatMember` 兼容别名

| 功能 | 实现数 | 状态 |
|------|--------|------|
| BotFather | 14/14 | ✅ 完整 |
| 消息发送 | 20+ | ✅ 完整 |
| 文件管理 | 5+ | ✅ 完整 |
| Webhook | 3/3 | ✅ 完整 |
| 业务消息 | 15+ | ✅ 完整 |
| **条件性未实现** | **若干请求分支** | ⚠️ 服务未配置或字段无法由 Layer 229 表达时返回方法级 501 |

基础 `sendGift`、`giftPremiumSubscription`、`transferBusinessAccountStars` 已具备支付表单校验和结算路径；`sendGift(pay_for_upgrade=true)` 与 `upgradeGift(star_count>0)` 因当前支付适配器无法承载升级 invoice 而返回标准 Telegram `501 Not Implemented`。支付、Stars 或目标服务未配置时，其它结算请求也会返回标准 `501`。

**设计决定**: 对无法安全表达或执行的请求返回标准 Telegram `501 Not Implemented`，避免静默丢字段或返回成功占位。

### 1.4 持久化完整性（局部/隔离证据）

PostgreSQL、事务和 outbox 相关检查已有局部或隔离证据，但尚未形成覆盖 813 个方法的生产级持久化、重启和跨设备验收。

**事务保障**
- ✅ 消息创建与投递在同一事务
- ✅ Outbox 写入原子性
- ✅ 已有隔离路径重启后无消息丢失证据（不等价于全量生产恢复）
- ✅ 重复消息自动去重

**迁移文件完整性**
- ✅ 99 个迁移文件，编号 001-101 连续
- ✅ 最近 5 个迁移覆盖核心功能
  - 097: 支付恢复
  - 098: 业务消息操作
  - 099: 投递终止状态
  - 100: 重放审计
  - 101: 频道访问控制

**DAO 层实现**
- ✅ PostgreSQL 连接池配置（16-48 连接/服务）
- ✅ 显式事务管理
- ✅ 错误传播和日志记录

### 1.5 文档完整性 ✅ PASS

| 文档 | 质量 | 覆盖范围 |
|------|------|---------|
| [AGENTS.md](AGENTS.md) | 高 | PostgreSQL 18、Layer 229 兼容性要求 |
| [BOT_API_COMPATIBILITY.md](BOT_API_COMPATIBILITY.md) | 高 | 185/185 路由和具体 adapter、已知限制 |
| [LAYER229_METHOD_LEDGER.csv](LAYER229_METHOD_LEDGER.csv) | 高 | 813 行审计记录 |
| [PERFORMANCE_OPTIMIZATION_SUMMARY.md](PERFORMANCE_OPTIMIZATION_SUMMARY.md) | 高 | 性能基线、优化历程 |
| [.env.example](.env.example) | 中 | 6 个必需变量 |
| README.md | ⚠️ | 需补充快速启动步骤 |

---

## 二、未完成的生产验收门槛

以下项目仍阻塞 `813/813` 完整生产验收。它们不是协议目录缺失，而是生产证据尚未闭环。

### 2.1 性能指标

当前数值来自隔离 fan-out 基线，不能作为长期生产 SLO 承诺。

**当前基线**（5000 用户 fan-out 隔离测试）

| 指标 | 目标 | 当前 | 状态 |
|------|------|------|------|
| ACK 延迟 | < 1s | 38-85ms | ✅ 达标 |
| Live P50 | < 2s | 1.1-1.4s | ✅ 达标 |
| Live P95 | < 2s | 2.9-4.7s | ⚠️ 未达标 |
| Live P99 | - | 3.9-7.0s | ⚠️ 未达标 |
| 连接池等待 | - | -71.7% | ✅ 改善 |

**结论**: ACK 路径在隔离基线中达标，但 live fan-out 的 P95/P99 未达目标；生产 SLO 尚未验收。

### 2.2 功能限制

| 限制项 | 原因 | 影响 |
|--------|------|------|
| Bot API 条件性缺口 | 官方方法已进入路由和 dispatch，但部分服务未配置或字段无法由 Layer 229 表达 | 返回方法级 501，避免成功占位 |
| 动态服务缺口 | RPC/DFS/支付等服务未配置时 | 返回服务配置 501 |
| 礼物付费升级 | `sendGift(pay_for_upgrade=true)` 和 `upgradeGift` 的正数 `star_count` 需要升级 invoice，当前 APIFull 支付适配器尚未承载 | 返回方法级 501，避免错误扣款或成功占位 |
| 消息扩展字段 | ordinary outbox 无 `message_effect_id` 字段；direct topic、ephemeral parameters、suggested post 尚无等价 adapter | 返回方法级 501，避免静默丢弃 |
| Rich draft stop controls | 当前 Layer 229 draft 记录无对应 flags | 返回方法级 501 |

### 2.3 环境依赖

- 🔧 PostgreSQL 18（必需，不支持 MySQL 5.7）
- 🔧 Redis（Session challenge 存储）
- 🔧 SeaweedFS S3（DFS 文件存储；生产使用 HA S3 origin）
- 🔧 Kafka（Sync 消息队列，已在 docker-compose 中）

### 2.4 必须补齐的生产证据

| 验收项 | 当前缺口 | 通过条件 |
|---|---|---|
| 计划消息（scheduled messages） | 未完成到期执行、重试/租约、重复触发、重启后的恢复和更新投递验收 | 在 PostgreSQL 18 + Kafka 的部署栈中完成端到端创建、到期执行、重试、重启恢复和客户端更新检查 |
| Provider 权威服务 | 接口/客户端与隔离 fixture 已存在；生产 Compose 模板中的多项 provider endpoint 默认空，实际环境覆盖值和对应日志未纳入本次验收。Identity 另有单机 staging 证据，详见分类清单 | 按[Provider 归属与验收状态](docs/provider-ownership-and-readiness-zh.md)为每项启用能力确定自有权威方或外部上游，再以目标环境凭据验证授权、签名、幂等、恢复和审计日志 |
| 媒体链路 | DFS 使用 S3 契约、默认自建 SeaweedFS；CDN 和群通话控制接口已存在。全链路上传下载、完整性、过期、真实 WebRTC/TURN 和生产故障恢复证据未闭环 | 在目标部署中验收实际选定的 S3、CDN、SFU/TURN 实现；这些可自建或托管，记录上传下载、重试、校验、授权、恢复和清理证据 |
| 重启恢复 | 多数方法缺少服务、session、outbox/consumer、authsession 的重启后读回和重放证明 | 对每个相关方法执行有界重启故障注入，验证无丢失、无重复副作用、状态可继续和更新可重放 |
| 跨设备 fan-out | 隔离会话证据不能覆盖真实多设备、断线重连、gap recovery 和 `pts/qts/seq` 连续性 | 至少两个独立设备/会话完成消息、已读、编辑、删除和更新补偿，并核对状态序列 |
| 真实流量 | 尚无 staging/production 代表性流量和 24 小时运行证据 | 用脱敏、可回滚的代表性流量完成限流、错误率、DLQ、数据库池和恢复演练，保留时间窗口指标 |
| 长期 SLO | 仅有短时隔离基线；P95/P99 和持续可用性未验收 | 完成至少一个长期 soak 窗口，按约定 P50/P95/P99、错误预算、DLQ、恢复时间和数据一致性门槛判定 |

在以上证据全部落地前，`21/21` 隔离 Web 通过和 `813/813` 目录匹配都只能记为阶段性证据。

---

## 三、生产部署清单

### Phase 1: 环境准备

```bash
# 1. 复制环境配置
cp .env.example .env

# 2. 调整 PostgreSQL 连接数（按副本数）
# 单实例: TEAMGRAM_POSTGRES_MAX_CONNECTIONS=256
# 多副本: 按 (副本数 × 224) + 40 迁移/运维连接计算

# 3. 启动基础栈
docker compose -f docker-compose-env.yaml up -d postgres redis seaweedfs kafka

# 4. 验证迁移完成
docker compose -f docker-compose-env.yaml exec postgres psql -U teamgram -d teamgram -c "SELECT COUNT(*) FROM schema_migrations;"
# 预期输出: count=99
```

### Phase 2: 启动应用

```bash
# 1. 启动所有服务
docker compose -f docker-compose-env.yaml -f docker-compose.yaml up -d

# 2. 等待 30 秒健康检查
sleep 30

# 3. 验证服务状态
docker compose -f docker-compose-env.yaml -f docker-compose.yaml ps --format json | jq '.[] | select(.State!="running")'
# 无输出 = 所有服务运行中

# 4. 检查 Prometheus 指标
curl -s http://localhost:9090/metrics | grep -c "^teamgram_"
# 预期: > 100 个指标
```

### Phase 3: 上线前验证清单

- [ ] PostgreSQL max_connections 已调整
- [ ] Redis 连接正常（PING）
- [ ] SeaweedFS S3 存储空间充足（> 100GB；生产按故障域和副本策略验收）
- [ ] Kafka brokers 全部在线
- [ ] Prometheus scrape 配置完成
- [ ] Grafana dashboard 加载正常
- [ ] 日志系统就位（ELK/Loki）

### Phase 4: 上线后监控

**关键指标**（建议告警阈值）

```promql
# 连接池使用率
teamgram_sync_postgres_pool_connections{state="acquired"} / teamgram_sync_postgres_pool_max_connections > 0.8

# 消息延迟 P95
histogram_quantile(0.95, rate(teamgram_apifull_channel_delivery_seconds_bucket[5m])) > 5

# Outbox 队列长度
teamgram_apifull_outbox_queue_size > 1000

# 数据库连接等待
rate(teamgram_sync_postgres_pool_empty_acquire_count[5m]) > 1

# 认证失败率
rate(teamgram_auth_failed_total[5m]) > 0.01
```

---

## 四、性能优化建议

### 当前已实现
- ✅ 异步 channel fan-out（ACK 不再阻塞投递）
- ✅ 安全提交批量（Kafka record age 明显下降）
- ✅ 并行 session 推送（最多 8 个并发 worker）
- ✅ Sync 回执批处理（最多 64 条/事务）
- ✅ PostgreSQL 连接池优化（MaxConns: 16）

### 后续优化方向（非阻塞）
1. **Auth Key 缓存**: 当前每次请求查询 DB，建议加 Redis 缓存
2. **消息预加载**: N+1 查询优化（批量预加载用户信息）
3. **Kafka 分片**: 当前 6 分区，按吞吐量调整
4. **Gateway 缓冲**: Response writer 有界缓冲

---

## 五、验收签字

| 项目 | 状态 | 日期 | 备注 |
|------|------|------|------|
| 功能正确性 | ⚠️ PARTIAL | 2026-10-20 | 21/21 隔离 Web 用例通过；不能外推为 813 个方法的生产通过 |
| Layer 229 兼容性 | ⚠️ NOT COMPLETE | 2026-10-20 | 813/813 目录匹配；完整生产验收 0/813，分层统计见 1.2 |
| Bot API 合约 | ✅ PARTIAL PASS | 2026-10-20 | 185/185 路由和具体 adapter；服务配置、不可表达字段和真实 E2E 仍有条件性限制 |
| 持久化完整性 | ⚠️ PARTIAL | 2026-10-20 | PostgreSQL 事务、outbox、幂等性有局部证据；重启和跨设备生产证据未闭环 |
| 性能基线 | ⚠️ NOT ACCEPTED | 2026-10-20 | 隔离基线已记录，P95/P99 和长期生产 SLO 未验收 |
| 文档完整性 | ✅ PASS | 2026-10-20 | 目录、ledger、限制项和未完成门槛已记录 |

**总体决议**: ⚠️ **协议目录完整，生产验收未完成；禁止声明 `813/813 COMPLETE`**

**有效期**: 2026-10-20 - 2026-12-20（完成第 2.4 节后重新验收）

**关键条件**:
1. 完成第 2.4 节的计划消息、Provider、媒体、重启恢复和跨设备 fan-out 证据
2. 按实际负载运行受控流量和长期 SLO/错误预算验收
3. 部署后激活关键指标、DLQ、数据库池和恢复告警
4. 重新生成严格报告，并确认所有 813 行达到生产验收门槛后，才可改为 COMPLETE

---

## 六、后续工作

### 短期（1-2 周）
- [ ] 完成第 2.4 节列出的计划消息、Provider 目标环境、媒体、重启恢复和跨设备 fan-out 验收
- [ ] 补充 README.md 快速启动指南
- [ ] 新建 KNOWN_LIMITATIONS.md 详细说明
- [ ] 准备 docker-compose production 配置模板
- [ ] 部署 Prometheus + Grafana + AlertManager

### 中期（1-3 月）
- [ ] 按实际负载重新压测 fan-out 性能
- [ ] 实现 Auth Key Redis 缓存
- [ ] 优化 N+1 查询模式
- [ ] 用真实支付、Stars、DFS 和 PostgreSQL 18 fixture 验证礼物、Premium 和 Business Stars 方法，并关闭剩余条件性 501 分支

### 长期（3-6 月）
- [ ] Kafka 分片优化
- [ ] 消息分片存储（按时间）
- [ ] 多数据中心支持
- [ ] 与 Telegram 官方对标测试

---

## 验收负责人

**主控**: Codex 续验
**执行方式**: 严格 ledger 校验 + 隔离 Web E2E + 现有探针证据分析
**本次记录**: 2026-10-20；完整生产验收未完成

**下一步**: 按第 2.4 节补齐生产证据，再运行严格报告进行复验。

---

**档案编号**: PROD-ACCEPT-20261017-TEAMGRAM-V1  
**保存位置**: [PRODUCTION_READINESS_DECISION.md](PRODUCTION_READINESS_DECISION.md)
