# Provider 归属与验收状态

更新日期：2026-10-20

## 判定口径

代码中的 `provider` 通常表示 Teamgram 调用某项能力的接口，不代表接口后面一定是第三方，也不代表已经部署了正式服务。需要分别确认：

1. 谁维护 Teamgram 侧的接口和适配器。
2. 谁持有最终权威数据、账号、支付交易或媒体会话。
3. 当前证据来自代码/fixture、本地 staging，还是目标生产环境。

仓库中的生产 Compose 配置给多项 provider endpoint 留有环境变量，但默认值为空。这能证明配置入口存在，不能证明运行环境已经注入了 endpoint 或凭据。以下状态依据当前代码、部署模板和验收记录；没有读取生产秘密或推断未展示的运行时覆盖值。

## 能力与归属

| 能力 | 自有部分 | 可能的外部上游 | 当前证据与缺口 |
|---|---|---|---|
| Product Identity | 独立 `services/identity` 拥有 `acct:<UUID>` 账号、JWT/JWKS、refresh session 和 Teamgram 账号绑定；Teamgram 有加密 MTProto proof 换 Identity JWT 的 bridge | Apple、Google、Microsoft OIDC（按启用项）；真实邮件发送依赖 SMTP/邮件服务 | 2026-10-19 有单机 Docker Desktop 上的双 Identity/AIMemory 实例、共享 Redis、重启和 key rotation staging 记录。该轮未覆盖外部 SMTP/OIDC、多主机故障域或生产 canary。Teamgram bridge 已做隔离验证，但不因此证明 Identity 是 Layer 229 `TEAMGRAM_AUTH_PROVIDER_*` 的后端 |
| Teamgram 登录、Bot 与 Passport | Teamgram 拥有 MTProto authsession、User/Bot registry 和 PostgreSQL 状态；Auth、Passport、Bot connection 有独立签名 provider 契约和配置入口 | 可由产品自有的凭据/安全资料服务承载；若某条能力依赖其他平台，以该能力的实际权威方为准 | `identity-provider-fixture` 与若干真实 Layer 229 session 探针证明隔离契约。`TEAMGRAM_AUTH_PROVIDER_*` 校验 Bot API 凭据和 Telegram Web token，不负责 `acct:<UUID>`、Identity JWT 或账号绑定。正式 authority、生产凭据及 durable confirmation/恢复证据未闭环 |
| 短信和邮件验证码 | Teamgram 有 challenge、限流和 HTTP delivery 接口；Identity 有邮件 challenge/outbox | 短信运营商/聚合商、SMTP 或邮件投递服务；也可以由自有网关转接 | `code-provider-fixture` 只把验证码保存在内存供测试读取，明确没有真实短信或邮件投递证据。Identity staging 记录也未使用外部 SMTP |
| Bot/Business/广告与资料控制面 | Teamgram 有 Business、Sponsored、User Channel Profiles 等 provider 客户端/契约；部分目录和业务状态可由 PostgreSQL 持有 | Fragment 等平台能力在相应方法启用时有自己的外部权威；广告库存也可由自有服务维护 | Business、Sponsored 等隔离 fixture 和部分 session probe 已存在。fixture/本地 projection 不能证明外部平台交易或生产 inventory authority；`USERCHANNELPROFILES` 等 endpoint 即使在部署模板中有配置项，也须单独提供运行环境证据 |
| 支付、Stars 与权益 | Teamgram/APIFull 拥有支付请求幂等、PostgreSQL ledger、权益投影、退款 webhook 接收和 reconciliation worker；有签名 Payment Provider 契约 | Apple App Store、Google Play，以及选用的银行卡/钱包/结算机构；交易、扣款、退款的最终权威由对应支付平台提供 | `payment-provider-fixture` 有签名、幂等、结算/退款和重启场景；这不是实际扣款或 store authority。Stripe、PayPal、Alipay 出现在 fixture 测试目录，不表示已接入这些商户服务。真实 receipt、webhook/refund 和结算凭据未验收 |
| 文件、对象存储与 CDN | DFS 使用标准 S3 API；默认可自建 SeaweedFS，也可部署其他自有 S3 服务。Files BFF 有签名 CDN provider 客户端 | 托管 S3、商业 CDN 可作为部署选择，不是接口的硬性依赖 | 本地单节点 SeaweedFS/S3 数据面探针和 CDN fixture session 证明部分链路。生产模板要求配置 S3 endpoint，CDN endpoint 默认空；目标生产的 HA、跨故障域恢复、真实 CDN 上传/下载与长期流量证据仍需单独验收 |
| 群通话与直播媒体 | APIFull 有签名媒体控制 provider 契约；TURN 凭据可由 Teamgram 配置/签发 | SFU、TURN relay、RTMP ingest 可自建或采购托管服务 | group-call media fixture 证明控制面请求形状，不证明真实音视频。当前验收记录未覆盖真实 WebRTC/TURN 双向媒体、SFU 故障切换和生产资源释放 |
| AI 模型 | Teamgram 有签名 provider 模式和 OpenAI-compatible adapter；endpoint 可指向自建模型服务 | OpenAI-compatible 云 API 或其他模型供应商 | 有适配器和配置项；provider 选择、生产凭据、模型效果、容量和降级策略须按实际目标服务验收。它不是全部 Telegram API 方法的生产验收前置条件 |

## 如何解释 0/813

`0/813` 表示当前 Layer 229 方法台账中没有方法登记为满足**完整生产验收**，不表示仓库没有自有 provider、没有隔离会话证据，或每项能力都必须购买第三方服务。方法只在其适用的权威服务、真实 session/update、重启恢复、生产流量和 SLO 门槛均有可复核证据后，才可以计为生产通过。

对可自建的能力（Identity、S3、CDN、AI、媒体控制面等），可使用自有服务满足 provider 门槛；对依赖外部交易或身份平台的能力，必须验收相应上游的真实权威行为。Fixture 只用于接口和隔离链路验收，不能代替任一方的生产证据。

## 相关文档

- [Teamgram to Identity bridge](identity-aimemory-integration.md)
- [Identity/AIMemory staging acceptance](identity-aimemory-staging-acceptance-20261019.md)
- [Authorization Provider contract](auth-provider.md)
- [Payment Provider contract](payment-provider.md)
- [Group Call Media Provider contract](group-call-media-provider.md)
- [Object storage and S3](object-storage-zh.md)
- [Verification-code fixture](code-provider-fixture.md)
- [AI Provider configuration](ai-provider.md)
- [Layer 229 critical-path acceptance](layer229-critical-path-acceptance.md)
