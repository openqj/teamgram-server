# Repository Map

本文件是 Teamgram Server 仓库的导航索引，面向开发者和 AI 助手。它描述模块职责、关键入口、公共契约和验证位置，而非每个文件的逐行摘要。代码变化后，优先更新模块边界和关键入口；普通 handler 或测试文件不需要逐项登记。

**索引时间：** 2026-10-17  
**Git 根目录：** `/Users/mac/open/test/teamgram-server`  
**Go 模块：** `github.com/teamgram/teamgram-server`

---

## 快速路由

| 任务类型 | 先读 | 主要代码位置 | 验证位置 |
|---------|------|-------------|---------|
| MTProto 方法、Layer 229 兼容性 | `AGENTS.md`、`BOT_API_COMPATIBILITY.md`、`LAYER229_FUNCTIONAL_AUDIT.md` | `app/bff/*/internal/core/*_handler.go` | `*_test.go`、`LAYER229_METHOD_LEDGER.csv` |
| PostgreSQL DAO、事务、迁移 | `AGENTS.md`、`README.md` | `app/**/internal/{dao,persist}/`、`pkg/storage/postgres/` | `teamgramd/deploy/sql/postgres/`、`*_postgres_test.go` |
| 消息投递、同步、更新推送 | `docs/communication-reliability.md`、`docs/messaging-reliability-operations.md` | `app/messenger/`、`app/service/biz/{message,dialog,updates}/` | `*_test.go`、可靠性探针 |
| Bot API / HTTP 接口 | `BOT_API_COMPATIBILITY.md` | `app/interface/httpserver/`、`app/botapi/` | `bot_api_*_test.go` |
| 服务部署、配置、监控 | `docs/service-topology.md`、`README-env-cn.md` | `teamgramd/etc2/`、`docker-compose*.yaml` | 部署探针、Prometheus 配置 |
| 性能优化、容量规划 | `PERFORMANCE_OPTIMIZATION_SUMMARY.md`、`docs/high-performance-components.md` | `pkg/`、高性能组件实现 | 性能测试、容量探针 |
| 生产就绪检查 | `PRODUCTION_READINESS_DECISION.md`、`PRODUCTION_ACCEPTANCE_PLAN.md` | 关键路径服务 | `acceptance-execution-plan.md` |

---

## 顶层结构

### `app/` — 应用服务

核心业务逻辑和服务入口，按职责分为 BFF、核心服务、消息处理和外部接口。

#### `app/bff/` — MTProto 边界前端服务（31 个服务）

每个 BFF 服务对应一组 Telegram API 方法，负责 MTProto 协议处理、参数验证、权限检查和 RPC 调用编排。

**标准目录结构：**
- `cmd/` — 服务启动入口
- `etc/` — YAML 配置文件
- `client/` — RPC 客户端（供其他服务调用）
- `internal/core/` — MTProto 方法 handler（`methodName_handler.go`）
- `internal/dao/` 或 `internal/persist/` — 数据访问层
- `internal/domain/` — 业务领域模型（部分服务有）
- `*_test.go` — 单元和边界测试

**主要 BFF 服务：**
- `account/` — 账户管理（删除账户、重置授权、隐私设置）
- `apifull/` — 完整 API 支持（表情、贴纸、礼物、星标、内联 bot、故事）
- `authorization/` — 授权流程（登录、登出、二维码、Passkey）
- `chats/` — 群组和频道管理
- `contacts/` — 联系人管理
- `dialogs/` — 对话列表
- `messages/` — 消息收发、编辑、删除
- `users/` — 用户信息和状态
- `updates/` — 更新订阅和状态同步
- `files/` — 文件上传下载
- 其他：`chatinvites/`、`configuration/`、`drafts/`、`notification/`、`premium/`、`usernames/` 等

#### `app/service/` — 核心业务服务

不直接暴露 MTProto 接口，为 BFF 提供可复用的业务能力。

- `authsession/` — 授权会话管理（session 生命周期、设备管理）
- `biz/` — 核心业务逻辑
  - `chat/` — 群组业务
  - `dialog/` — 对话业务
  - `message/` — 消息业务
  - `updates/` — 更新生成和分发
  - `user/` — 用户业务
- `idgen/` — 全局 ID 生成
- `media/` — 媒体处理
- `dfs/` — 分布式文件存储
- `status/` — 在线状态
- `geoip/` — 地理位置

#### `app/messenger/` — 消息管线

- `msg/` — 消息处理核心
- `msg/inbox/` — 收件箱处理
- `sync/` — 会话同步、更新推送、投递确认

#### `app/interface/` — 外部接口层

- `httpserver/` — Bot API / HTTP 服务
  - `internal/server/http/` — HTTP 路由和 handler
  - `internal/logic/` — Bot API 业务逻辑
- `session/` — MTProto 会话入口
- `gnetway/` — 网络网关

#### `app/botapi/` — Bot API 支持

Bot API 类型定义、存储、序列化和辅助代码。HTTP 路由实现在 `app/interface/httpserver/`。

---

### `pkg/` — 共享库

跨服务的可复用组件，提供基础能力而不包含业务逻辑。

**核心包：**
- `storage/postgres/` — PostgreSQL 连接池、事务、DAO 基类
- `mtprotoext/` — MTProto 协议扩展和辅助工具
- `rpc/` — RPC 客户端和服务端框架
- `mqconsumer/`、`mqproducer/` — 消息队列（Kafka/NATS）
- `providercontract/` — 提供者契约接口（AI、支付、通信等）
- `code/` — 短信验证码生成
- `ai/` — AI 能力集成
- `twofa/` — 两步验证
- `phonenumber/` — 电话号码解析
- `mention/` — @提及解析
- `deduplication/` — 去重逻辑
- `hashx/`、`net2/`、`httpx/`、`keyedexecutor/` — 工具库

---

### `teamgramd/` — 部署和运行时

**关键子目录：**
- `etc2/` — 服务配置文件（YAML，按服务组织）
- `deploy/` — 部署资源
  - `sql/postgres/` — **PostgreSQL 迁移脚本**（权威）
  - `prometheus/` — Prometheus 监控配置
  - `postgres/` — PostgreSQL 容器配置
  - `alertmanager/`、`filebeat/`、`go-stash/` — 告警和日志
- `bin/` — 本地构建输出（.gitignore）
- `logs/` — 运行日志（.gitignore）
- `docker/` — Docker 相关配置
- `third_party/` — 第三方依赖

---

### `data/` — 数据和初始化

- `postgres/` — PostgreSQL 18 相关数据
  - 实际生产迁移以 `teamgramd/deploy/sql/postgres/` 为准

---

### `docs/` — 运维和可靠性文档

**关键文档：**
- `service-topology.md` / `service-topology-zh.md` — 服务拓扑图
- `communication-reliability.md`、`messaging-reliability-operations.md` — 消息可靠性
- `layer229-critical-path-acceptance.md` — Layer 229 关键路径验收
- `partial-probe-evidence.md` — 探针证据
- `install-manual-*.md` — 手动安装指南（Linux、macOS）
- `service-monitoring.md`、`log-collection.md` — 监控和日志
- `*-provider.md` — 提供者契约文档（AI、支付、通信、验证码）

---

### `specs/` — 架构和规范

**核心规范：**
- `architecture.md` / `architecture-zh.md` — 架构设计
- `protocol-and-compatibility.md` — 协议和兼容性
- `dependencies-and-runtime.md` — 依赖和运行时
- `high-performance-components.md` — 高性能组件
- `roadmap.md`、`release-and-changelog.md` — 路线图和发布
- `security.md`、`contributing.md` — 安全和贡献指南

---

### `tools/` — 工具和探针

验证工具、容量测试、Bot API 探针、群组 fanout 测试等。

---

### `clients/` — 客户端兼容性

兼容客户端列表和补丁说明。

---

### `docker/` — 容器配置

容器化部署辅助。主要编排文件在根目录的 `docker-compose*.yaml`。

---

### `cmd/` — 命令行工具

独立的 CLI 工具入口。

---

## 根目录关键文档

| 文档 | 用途 |
|-----|------|
| `AGENTS.md` | **项目指令**：协议兼容、PostgreSQL 目标、范围控制（优先级最高） |
| `README.md` / `README-zh.md` | 项目介绍和快速开始 |
| `BOT_API_COMPATIBILITY.md` | Bot API 兼容性状态和测试 |
| `LAYER229_FUNCTIONAL_AUDIT.md` | Layer 229 功能审计（617KB，详尽） |
| `LAYER229_METHOD_LEDGER.csv` | Layer 229 方法实现清单 |
| `PERFORMANCE_OPTIMIZATION_SUMMARY.md` | 性能优化总结 |
| `PRODUCTION_READINESS_DECISION.md` | 生产就绪决策 |
| `PRODUCTION_ACCEPTANCE_PLAN.md` | 生产验收计划 |
| `acceptance-execution-plan.md` | 验收执行计划 |
| `README-env-cn.md` / `README-env-en.md` | 环境变量配置说明 |
| `.env.example` | 环境变量模板 |

---

## 后端开发流程

处理一个 MTProto 方法或业务功能时，按以下顺序：

1. **查找契约** — 在 `LAYER229_METHOD_LEDGER.csv` 或 `telegram-api/schema/telegram_api_layer229.tl`（外层仓库）查找方法签名和返回类型
2. **定位 Handler** — 在 `app/bff/*/internal/core/` 找到对应的 `methodName_handler.go`
3. **追踪 RPC 调用** — 沿着 handler 的 RPC 客户端调用进入 `app/service/` 或 `app/messenger/`
4. **检查持久化** — 查看 `internal/dao/` 或 `internal/persist/`，以及 PostgreSQL 迁移
5. **运行测试** — 先运行 `*_test.go`，特别是 `*_postgres_test.go` 和边界测试
6. **验证兼容性** — 对照 `BOT_API_COMPATIBILITY.md` 或 Layer 229 文档

**命名线索：**
- `*_handler.go` — MTProto 方法入口
- `*_test.go` — 单元测试
- `*_postgres_test.go` — PostgreSQL 集成测试
- `cmd/*/main.go` — 服务启动入口
- `etc/*.yaml` — 服务配置
- `client/*.go` — RPC 客户端
- `internal/dao/` — 数据访问对象
- `internal/persist/` — 持久化层

---

## PostgreSQL 迁移

**权威位置：** `teamgramd/deploy/sql/postgres/`

生产环境必须使用 PostgreSQL 18。新服务启动时必须：
1. 要求非空 PostgreSQL DSN
2. 创建连接池并 ping 验证
3. 空或不可用的数据库必须导致启动失败（不得降级到 MySQL 或 nil DAO）

---

## 忽略的目录

以下目录默认不纳入上下文（需要时单独检查）：

- `teamgramd/bin/`、`teamgramd/logs/` — 构建输出和日志
- `teamgramd/third_party/` — 第三方依赖
- `*.pb.go`、生成代码 — 除非涉及协议变更
- 大型二进制和资源文件

---

## 维护规则

1. **新增模块或服务** — 更新本文件的对应章节
2. **新增 PostgreSQL 迁移** — 确保记录在 `teamgramd/deploy/sql/postgres/`
3. **协议变更** — 同步更新 `LAYER229_METHOD_LEDGER.csv` 和兼容性文档
4. **普通文件增删** — 只要命名约定和模块边界不变，无需更新本文件
5. **验证** — 用 `git diff --check` 和相关测试验证变更

**冲突时：** 以 `AGENTS.md`、源代码、测试结果和实际 schema 为准。本文件是导航索引，不替代它们。

---

## 相关仓库

本仓库是 Teamgram 多仓库工作区的一部分：

- **外层工作区：** `/Users/mac/open/test/`
- **telegram-api/** — API Layer 229 TL 定义和方法索引
- **proto/** — MTProto 核心类型、编解码、gRPC 共享代码
- **teamgram-ios/** — iOS 客户端（Bazel）
- **telegram-tt-master/** — Web/Tauri 客户端（Vite + TypeScript）

跨仓库工作时，先查看外层的 `REPO_MAP.md` 和 `AGENTS.md`。
