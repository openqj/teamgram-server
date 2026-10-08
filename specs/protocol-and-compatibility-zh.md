# 协议与兼容性

本文档说明 Teamgram Server 所支持的 MTProto 子协议、API Layer 及与官方客户端的兼容范围。

## MTProto 2.0

Teamgram 实现 [MTProto 2.0](https://core.telegram.org/mtproto)，支持以下传输格式：

- **Abridged**
- **Intermediate**
- **Padded intermediate**
- **Full**

客户端通过 TCP 或 WebSocket 连接 gnetway，使用上述任一格式进行握手与 RPC。

## 协议边界与 API Layer

- 对外后端功能方法指 **MTProto User API**；内部 gRPC、DAL 和存储层可以使用项目内部契约，但对外适配必须保持 Telegram 语义。Bot API 与 TDLib 不在本项目兼容范围内，除非方法另有明确标注。
- 当前支持的 **API Layer** 为 **229**。
- TL 线协议以官方 Telegram TL 定义和仓库中的 [`telegram-api/schema/telegram_api_layer229.tl`](../../telegram-api/schema/telegram_api_layer229.tl) 为准；`proto/mtproto` 生成类型、手写 Layer 229 编解码补充和 `telegram-api` 方法定义必须与其一致。
- 标准方法必须保留 constructor ID、方法名、参数顺序、flags、可选字段、Vector 顺序和返回 constructor。升级 API Layer 时，必须同步更新 schema、生成代码、客户端兼容说明和测试，不能静默混用不同层。
- 方法行为必须匹配 Telegram 客户端对权限、实体 ID 与 `access_hash`、分页、时间戳、幂等性、标准 RPC 错误以及 `pts`、`qts`、`seq`、`date` 更新状态的预期。

## 客户端功能闭环与验收证据

- [LAYER229_METHOD_LEDGER.csv](../LAYER229_METHOD_LEDGER.csv) 是方法状态、`audit_status`、`production_acceptance_status` 与验收证据的唯一来源；[功能验收记录](../LAYER229_FUNCTIONAL_AUDIT.md)提供场景说明，避免另建一套状态台账。
- 在已有证据字段或其链接的验收说明中建立“客户端/版本与功能 → Layer 229 方法 → 权限及实体身份 → 权威持久化 → 更新同步与差分恢复 → 验收场景/结果”的映射。一个功能涉及多个方法时，逐项追踪必要方法及外部依赖。
- 对所需的聊天、频道/超级群、论坛、秘密聊天、搜索、通知、定时消息、媒体及通话分别完成闭环。范围以支持客户端的实际需求为准，Bot API 与 TDLib 的范围仍按上文约定。
- 验收覆盖 flags/可选字段、正确返回与错误分支、权限拒绝、方法规定的幂等性、多设备更新和重启恢复。正确 TL 类型或合法空结果只证明相应分支，不构成整个功能通过；未实现业务或缺失必要 provider 时不能虚报成功。
- 重连、重复/乱序更新、编辑删除和成员权限变化按[架构文档](architecture-zh.md)验收。通话还需真实双向媒体证据，媒体需标准文件路径与元数据/内容一致性证据；待验证项保留实际未验收状态。

## 兼容客户端

| 客户端 | 说明 |
|--------|------|
| [Android (teamgram-android)](../clients/teamgram-android.md) | 修改 ConnectionsManager.cpp 中的服务器地址与端口 |
| [iOS (teamgram-ios)](../clients/teamgram-ios.md) | 配置连接自建服务器 |
| [桌面端 (teamgram-tdesktop)](../clients/teamgram-tdesktop.md) | 配置连接自建服务器 |

**重要**：默认登录验证码为 **12345**（仅开发用；生产环境请修改）。

## 部署说明

- 客户端需能访问 gnetway 端口（默认 10443、11443、5222）。TLS 或反向代理（如 Nginx WebSocket）需按需配置。
