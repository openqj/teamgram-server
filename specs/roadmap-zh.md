# 路线图与目标

本文档描述短期与中期目标，以及社区版与企业版的功能边界。

## 项目定位与演进策略

Teamgram 是本项目当前采用的社区版开源基础。本阶段以 Teamgram 基础实现和 MTProto 协议为过渡基础，持续补齐后端功能与 Layer 229 方法，直到满足支持客户端的完整功能需求和上线要求。

在过渡阶段，Teamgram 的协议实现、TL 定义和兼容行为是公共 MTProto 边界的基础。新增或修复功能应优先保持客户端兼容，不能为了提前独立化而破坏现有线协议或移除必要依赖。

只有在客户端所需功能和方法完成兼容性及上线就绪验收后，才进入独立实现和逐步脱离 Teamgram 依赖的阶段。独立化必须保留公共 MTProto 合约和可迁移路径。

## 实施顺序与阶段出口

采用[架构文档](architecture-zh.md)中的渐进方案。以下顺序表示验收依赖；明确边界内的功能补齐与数据库迁移可交叉推进。上线前不需要历史数据搬迁，但必须验证 PostgreSQL 中的业务语义和故障恢复。

| 阶段 | 工作内容 | 阶段出口 |
|------|----------|----------|
| 1. 功能与性能基线 | 沿用 Layer 229 唯一方法台账，将客户端功能关联到权限、持久化、更新恢复和验收场景；确定在线人数、消息速率、群规模、媒体负载、地区/RTT 与硬件 | 方法缺口和证据可追踪；负载模型、测量边界及性能预算已明确，初始预算未被标记为实测结果 |
| 2. PostgreSQL 存储迁移 | 建立项目本地 PostgreSQL 数据访问层，转换 schema、DAO、原始 SQL、锁、幂等与初始化/迁移脚本；覆盖所有生产服务 | PostgreSQL 18 上的服务、事务、更新恢复、兼容探针及部署检查通过，才切换默认运行路径；生产数据访问收敛为 PostgreSQL |
| 3. 客户端功能闭环 | 完成支持客户端所需的聊天、频道、论坛、秘密聊天、搜索、通知、定时消息和通话等能力，逐项关联方法和实际依赖 | 权限、业务结果、持久化、多端更新及重启恢复有证据；通话和外部 provider 等能力经过真实客户端/集成验收，缺口继续如实记录 |
| 4. 关键路径优化 | 测量串行 RPC、逐成员投递和频道事务写入；完善 Outbox、批量投递与媒体资源隔离，按证据调整部署边界、索引及容量 | 同一负载模型下有优化前后结果；私聊、群、频道、媒体的 P95/P99、吞吐及积压满足所选目标，正确性不退化 |
| 5. 上线就绪 | 验证断线追赶、Worker 重试、进程退出、主库故障、备份恢复和资源释放；完成真实授权、推送与媒体等所需依赖配置 | 所需方法达到台账规定的生产验收状态，RPO/RTO 与容量有证据，并通过[发布验收](release-and-changelog-zh.md) |
| 6. 逐步独立化 | 在前述验收完成后按模块移除 Teamgram 依赖；对难以满足要求的模块单独评估重写 | 保持 Layer 229 公共合约和客户端行为；每次替换有兼容、恢复、性能证据及可回退或兼容迁移路径 |

阶段状态以实际证据更新。注册了方法、返回正确 constructor、编译通过或完成数据库选型均不能表示相应阶段已经完成。

### PostgreSQL 迁移状态（2026-10-08）

第一批 PostgreSQL 18 切片已经实现并验证。生产服务现在要求 PostgreSQL 配置；方法面尚未完成的服务必须启动失败，不能回退 MySQL：

| 切片 | 证据 | 运行时状态 |
|------|------|------------|
| 存储基础层 | `pgxpool`、事务/错误辅助函数及专项包测试 | 可供新适配器使用 |
| authsession schema 与 DAO | 全新 schema、独立 `pgx` DAO、`go test ./app/service/authsession/...` | 等 authsession 全部方法共用 PostgreSQL 路径后再接入 |
| 基础群组 chat schema 与 DAO | 全新 `chats`/`chat_participants` schema、独立 DAO、`go test ./app/service/biz/chat/...` | invite 及相关 chat 聚合迁移后再接入 |
| 更新状态 schema 与 DAO | 全新 `user_pts_updates`/`auth_seq_updates` schema、序列唯一约束、独立 `pgx` DAO、`go test ./app/service/biz/updates/...` | 更新写入与恢复读取共用 PostgreSQL 事务后再接入 |
| message 聚合 schema 与 DAO | 全新 `messages`、已读 outbox、hashtag schema，随机 ID 幂等写入，独立 `pgx` DAO、`go test ./app/service/biz/message/...` | 核心 DAO 可用；完整消息方法面和运行时接线仍待完成 |
| user 辅助与 media DAO 切片 | user privacy/presence/peer/settings DAO，以及全新 media 文档/照片尺寸 schema 与 DAO，专项包测试 | 核心持久化可用；完整方法覆盖和运行时接线仍待完成 |
| APIFull domain | PostgreSQL connector、CAS/advisory lock、36 表运行时 schema 转换、PostgreSQL 18 探针 | domain 路径可用；生产迁移和完整 CRUD 兼容性仍待验收 |
| 部署 | PostgreSQL 18 清库 Compose、带校验和的 migration runner、可移植性检查 | PostgreSQL 18 是生产部署基线 |

user、dialog、message、media、inbox、sync 及更新状态等服务仍需完成全部 handler 覆盖后才能通过生产验收。其生成的 MySQL DAO 只属于迁移边界代码，生产配置不得打开。

## 短期目标

- **文档与规范**：保持 specs（架构、协议、依赖、贡献、安全、发布、路线图）与代码一致。
- **数据库基线**：PostgreSQL 18 是独立实现的唯一生产数据库。当前从 Teamgram 的 MySQL DAO、SQL 和迁移脚本切换到 PostgreSQL；项目尚未上线，不需要历史数据迁移。生产服务必须在 PostgreSQL 路径未完成时直接阻止启动，不保留双数据库运行时。
- **CI 与质量**：引入 CI（如 GitHub Actions）做构建、测试、lint（如 golangci-lint）；在 Makefile 中增加 `test`、`lint`、`fmt` 等目标。
- **文档一致性**：统一 README 与各文档（如 docker-compose-env 命名、安装文档）。

## 中期目标

- **测试覆盖**：增加核心路径的单元测试与集成测试，逐步建立覆盖率要求。
- **可观测与运维**：补充监控栈（Prometheus、Grafana、Jaeger、ELK）使用说明与 runbook。
- **安装与部署**：覆盖更多安装场景（如其他 Linux 发行版、Kubernetes 示例），与 docker-compose-env 文档保持一致。

## 社区版与企业版边界

以下能力在**企业版**中提供，社区版不包含或仅部分支持：

- sticker / theme / chat_theme / wallpaper / reactions / secret chat / 2FA / SMS / push（APNS / Web / FCM）/ web / scheduled / autodelete / …
- channels / megagroups
- audio / video / group / conferenceCall
- bots
- miniapp

详见主 README「企业版」小节。社区版当前优先完成基础协议、客户端兼容和上线能力；企业版功能边界不改变本项目补齐客户端所需能力的演进目标。
